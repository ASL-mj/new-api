package model

import "testing"

func TestResolveModelMappingTarget(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		mapping string
		want    string
	}{
		{
			name:    "direct mapping",
			model:   "hy3",
			mapping: `{"hy3":"b/hy3"}`,
			want:    "b/hy3",
		},
		{
			name:    "chained mapping",
			model:   "public-model",
			mapping: `{"public-model":"provider-model","provider-model":"provider-model-v2"}`,
			want:    "provider-model-v2",
		},
		{
			name:    "invalid mapping falls back",
			model:   "public-model",
			mapping: `{invalid`,
			want:    "public-model",
		},
		{
			name:    "cycle falls back",
			model:   "public-model",
			mapping: `{"public-model":"provider-model","provider-model":"public-model"}`,
			want:    "public-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := tt.mapping
			if got := resolveModelMappingTarget(tt.model, &mapping); got != tt.want {
				t.Fatalf("resolveModelMappingTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetChannelModelForRequest(t *testing.T) {
	mapping := `{"hy3":"b/hy3"}`
	channel := &Channel{Models: "hy3", ModelMapping: &mapping}

	if got := GetChannelModelForRequest(channel, "b/hy3"); got != "hy3" {
		t.Fatalf("GetChannelModelForRequest() = %q, want %q", got, "hy3")
	}
	if got := GetChannelModelForRequest(channel, "unmapped"); got != "unmapped" {
		t.Fatalf("GetChannelModelForRequest() = %q, want %q", got, "unmapped")
	}
}
