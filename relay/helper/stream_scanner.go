package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10 // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 64 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
	streamWriteTimeout          = 30 * time.Second
	defaultClientGoneDrainLimit = 64
)

var (
	clientGoneDrainSlots     chan struct{}
	clientGoneDrainSlotsOnce sync.Once
)

func getClientGoneDrainSlots() chan struct{} {
	clientGoneDrainSlotsOnce.Do(func() {
		limit := constant.ClientGoneDrainLimit
		if limit <= 0 {
			limit = defaultClientGoneDrainLimit
		}
		clientGoneDrainSlots = make(chan struct{}, limit)
	})
	return clientGoneDrainSlots
}

func acquireClientGoneDrainSlot() bool {
	select {
	case getClientGoneDrainSlots() <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseClientGoneDrainSlot() {
	<-getClientGoneDrainSlots()
}

func clientGoneDrainTimeout() time.Duration {
	timeout := constant.ClientGoneDrainTimeout
	if timeout <= 0 {
		timeout = 900
	}
	return time.Duration(timeout) * time.Second
}

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

// ExtendWriteDeadline prevents a blocked client write from keeping stream
// cleanup waiting indefinitely.
func ExtendWriteDeadline(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
}

// discardResponseWriter keeps response-processing callbacks running after the
// downstream connection is gone without attempting another network write.
// Some adaptors write through c.Render directly rather than a helper method.
type discardResponseWriter struct {
	gin.ResponseWriter
}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *discardResponseWriter) WriteString(s string) (int, error) {
	return len(s), nil
}

func (w *discardResponseWriter) WriteHeader(_ int) {}

func (w *discardResponseWriter) WriteHeaderNow() {}

func (w *discardResponseWriter) Flush() {}

func (w *discardResponseWriter) Status() int {
	return http.StatusOK
}

func (w *discardResponseWriter) Size() int {
	return 0
}

func (w *discardResponseWriter) Written() bool {
	return true
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {

	if resp == nil || dataHandler == nil {
		return
	}

	// Preserve any status assembled before streaming, including prior soft errors.
	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	ctx, cancel := context.WithCancel(context.Background())

	var (
		stopChan       = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner        = bufio.NewScanner(resp.Body)
		ticker         = time.NewTicker(streamingTimeout)
		pingTicker     *time.Ticker
		writeMutex     sync.Mutex     // Mutex to protect concurrent writes
		wg             sync.WaitGroup // 用于等待所有 goroutine 退出
		cleanupOnce    sync.Once
		stopOnce       sync.Once
		disconnectOnce sync.Once
	)
	stop := func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	if common.DebugEnabled {
		// print timeout and ping interval for debugging
		println("relay timeout seconds:", common.RelayTimeout)
		println("relay max idle conns:", common.RelayMaxIdleConns)
		println("relay max idle conns per host:", common.RelayMaxIdleConnsPerHost)
		println("streaming timeout seconds:", int64(streamingTimeout.Seconds()))
		println("ping interval seconds:", int64(pingInterval.Seconds()))
	}

	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	cleanup := func() {
		cleanupOnce.Do(func() {
			cancel()
			stop()
			if resp.Body != nil {
				_ = resp.Body.Close()
			}

			ticker.Stop()
			if pingTicker != nil {
				pingTicker.Stop()
			}

			wg.Wait()
		})
	}
	defer cleanup()

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	markClientGone := func() {
		disconnectOnce.Do(func() {
			info.StreamStatus.MarkDownstreamDisconnected()
			writeMutex.Lock()
			c.Writer = &discardResponseWriter{ResponseWriter: c.Writer}
			writeMutex.Unlock()
		})
	}

	// The upstream request uses a context without the downstream cancellation
	// signal. When the client leaves, keep draining the established upstream
	// stream for a bounded period so final usage can be recorded and settled.
	wg.Add(1)
	gopool.Go(func() {
		defer wg.Done()
		if c == nil || c.Request == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-c.Request.Context().Done():
			markClientGone()
			logger.LogInfo(c, "downstream disconnected; continuing upstream stream for usage settlement")
		}

		if !acquireClientGoneDrainSlot() {
			err := fmt.Errorf("client gone drain concurrency limit reached")
			info.StreamStatus.RecordError(err.Error())
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			stop()
			return
		}
		defer releaseClientGoneDrainSlot()

		timer := time.NewTimer(clientGoneDrainTimeout())
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			err := fmt.Errorf("client gone drain timeout after %s", clientGoneDrainTimeout())
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGoneTimeout, err)
			stop()
		}
	})

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					stop()
				}
				if common.DebugEnabled {
					println("ping goroutine exited")
				}
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					if c.Request != nil && c.Request.Context().Err() != nil {
						markClientGone()
						return
					}
					writeMutex.Lock()
					ExtendWriteDeadline(c)
					err := PingData(c)
					writeMutex.Unlock()
					if err != nil {
						if c.Request != nil && c.Request.Context().Err() != nil {
							markClientGone()
							return
						}
						logger.LogError(c, "ping data error: "+err.Error())
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
						return
					}
					if common.DebugEnabled {
						println("ping data sent")
					}
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
			stop()
		}()
		sr := newStreamResult(info.StreamStatus)
		for data := range dataChan {
			sr.reset()
			writeMutex.Lock()
			if !info.StreamStatus.IsDownstreamDisconnected() {
				ExtendWriteDeadline(c)
			}
			dataHandler(data, sr)
			writeMutex.Unlock()
			if sr.IsStopped() {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			stop()
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			if common.DebugEnabled {
				println(data)
			}

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++

				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				if common.DebugEnabled {
					println("received [DONE], stopping scanner")
				}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
			}
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	})

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
	}

	cleanup()

	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}
