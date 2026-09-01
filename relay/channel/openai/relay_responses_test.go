package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type signalResponseWriter struct {
	gin.ResponseWriter
	wrote chan struct{}
	once  sync.Once
}

func (w *signalResponseWriter) notifyWrite() {
	w.once.Do(func() {
		close(w.wrote)
	})
}

func (w *signalResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.notifyWrite()
	return n, err
}

func (w *signalResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.notifyWrite()
	return n, err
}

func TestOaiResponsesStreamHandler_ClientGoneDrainsUpstreamAndUsesFinalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Writer = &signalResponseWriter{ResponseWriter: c.Writer, wrote: make(chan struct{})}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}
	info.SetEstimatePromptTokens(17112)

	result := make(chan *dto.Usage, 1)
	errs := make(chan error, 1)
	go func() {
		usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: reader})
		if err != nil {
			errs <- err
			return
		}
		result <- usage
	}()

	_, err := fmt.Fprint(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n")
	require.NoError(t, err)
	select {
	case <-c.Writer.(*signalResponseWriter).wrote:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first downstream SSE event")
	}

	cancel()

	// The downstream is gone, but the upstream still sends the final usage.
	go func() {
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"usage\":{\"input_tokens\":17112,\"output_tokens\":8361,\"total_tokens\":25473}}}\n")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n")
		_ = writer.Close()
	}()

	select {
	case err := <-errs:
		t.Fatalf("stream handler returned error: %v", err)
	case usage := <-result:
		require.Equal(t, 17112, usage.PromptTokens)
		require.Equal(t, 8361, usage.CompletionTokens)
		require.Equal(t, 25473, usage.TotalTokens)
		require.Empty(t, usage.UsageSource)
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not drain the upstream response")
	}
}

func TestOaiResponsesStreamHandler_ClientGoneSettlesInputOnlyFinalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Writer = &signalResponseWriter{ResponseWriter: c.Writer, wrote: make(chan struct{})}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}
	info.SetEstimatePromptTokens(21520)

	result := make(chan *dto.Usage, 1)
	errs := make(chan error, 1)
	go func() {
		usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: reader})
		if err != nil {
			errs <- err
			return
		}
		result <- usage
	}()

	_, err := fmt.Fprint(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_input_only\"}}\n")
	require.NoError(t, err)
	select {
	case <-c.Writer.(*signalResponseWriter).wrote:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first downstream SSE event")
	}

	cancel()
	go func() {
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_input_only\",\"usage\":{\"input_tokens\":21520,\"output_tokens\":0,\"total_tokens\":21520}}}\n")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n")
		_ = writer.Close()
	}()

	select {
	case err := <-errs:
		t.Fatalf("stream handler returned error: %v", err)
	case usage := <-result:
		require.Equal(t, 21520, usage.PromptTokens)
		require.Zero(t, usage.CompletionTokens)
		require.Equal(t, 21520, usage.TotalTokens)
		require.Empty(t, usage.UsageSource)
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not drain the upstream response")
	}
}

func TestOaiResponsesStreamHandler_ClientGoneWithoutFinalUsageKeepsAuditEstimate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Writer = &signalResponseWriter{ResponseWriter: c.Writer, wrote: make(chan struct{})}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "test-model",
		},
	}
	info.SetEstimatePromptTokens(21520)

	result := make(chan *dto.Usage, 1)
	go func() {
		usage, _ := OaiResponsesStreamHandler(c, info, &http.Response{Body: reader})
		result <- usage
	}()

	_, err := fmt.Fprint(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_without_usage\"}}\n")
	require.NoError(t, err)
	select {
	case <-c.Writer.(*signalResponseWriter).wrote:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first downstream SSE event")
	}

	cancel()
	require.Eventually(t, func() bool {
		return info.StreamStatus != nil && info.StreamStatus.IsDownstreamDisconnected()
	}, time.Second, 10*time.Millisecond)

	_, err = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	select {
	case usage := <-result:
		require.Equal(t, 21520, usage.PromptTokens)
		require.Greater(t, usage.CompletionTokens, 0)
		require.Equal(t, dto.UsageSourceEstimatedClientGone, usage.UsageSource)
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not finish after upstream EOF")
	}
}

func TestOaiResponsesStreamHandler_PreservesFinalZeroOutputUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	reader := strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_zero_output\",\"usage\":{\"input_tokens\":21520,\"output_tokens\":0,\"total_tokens\":21520}}}\ndata: [DONE]\n")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
		},
	}

	usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(reader)})
	require.Nil(t, err)
	require.Equal(t, 21520, usage.PromptTokens)
	require.Zero(t, usage.CompletionTokens)
	require.Equal(t, 21520, usage.TotalTokens)
}
