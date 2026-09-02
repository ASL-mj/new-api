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

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserSortClause(t *testing.T) {
	require.Equal(t, "(quota + used_quota) asc, id asc", userSortClause("total_quota", "asc"))
	require.Equal(t, "(quota + used_quota) desc, id desc", userSortClause("total_quota", "desc"))
	require.Equal(t, "aff_count asc, id asc", userSortClause("aff_count", "asc"))
	require.Equal(t, "id desc", userSortClause("quota", "asc"))
	require.Equal(t, "id desc", userSortClause("total_quota", "invalid"))
}
