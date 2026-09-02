/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetUserModelGroups(t *testing.T) {
	groups := map[string]string{
		"auto":    "自动分组",
		"default": "默认分组",
	}

	require.Equal(t, []string{"default"}, getUserModelGroups("", "auto", groups))
	require.Equal(t, []string{"default"}, getUserModelGroups("", "default", groups))
	require.Empty(t, getUserModelGroups("", "auto", map[string]string{"default": "默认分组"}))
	require.Empty(t, getUserModelGroups("", "missing", groups))
}
