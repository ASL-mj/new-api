/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJoinBaseURLPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		endpoint string
		want     string
	}{
		{
			name:     "no trailing slash",
			baseURL:  "https://example.com",
			endpoint: "/v1/models",
			want:     "https://example.com/v1/models",
		},
		{
			name:     "one trailing slash",
			baseURL:  "https://example.com/",
			endpoint: "/v1/models",
			want:     "https://example.com/v1/models",
		},
		{
			name:     "multiple trailing slashes",
			baseURL:  "https://example.com///",
			endpoint: "v1/models",
			want:     "https://example.com/v1/models",
		},
		{
			name:     "base path is preserved",
			baseURL:  "https://example.com/api/",
			endpoint: "/v1/models",
			want:     "https://example.com/api/v1/models",
		},
		{
			name:     "protocol slashes are preserved",
			baseURL:  " https://example.com ",
			endpoint: " /v1/models ",
			want:     "https://example.com/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JoinBaseURLPath(tt.baseURL, tt.endpoint))
		})
	}
}
