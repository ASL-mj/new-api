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

For commercial licensing, please contact support@quantumnous.com
*/

export const SUMMARY_CACHE_TTL_MS = 60 * 1000;

export function createModelPerformanceSummaryStore({
  fetchSummary,
  now = Date.now,
  ttlMs = SUMMARY_CACHE_TTL_MS,
}) {
  const cache = new Map();
  const inFlight = new Map();

  const toModelMap = (data) =>
    new Map(
      (data?.models || [])
        .filter((item) => item?.model_name)
        .map((item) => [item.model_name, item]),
    );

  const load = (hours = 24) => {
    const cached = cache.get(hours);
    if (cached && now() - cached.fetchedAt < ttlMs) {
      return Promise.resolve(cached.models);
    }
    if (inFlight.has(hours)) {
      return inFlight.get(hours);
    }

    const request = Promise.resolve(fetchSummary(hours))
      .then((data) => {
        const models = toModelMap(data);
        cache.set(hours, { models, fetchedAt: now() });
        return models;
      })
      .finally(() => {
        if (inFlight.get(hours) === request) {
          inFlight.delete(hours);
        }
      });
    inFlight.set(hours, request);
    return request;
  };

  return { load };
}
