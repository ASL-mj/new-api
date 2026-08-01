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
export const parseChannelModels = (value) => {
  const seen = new Set();
  return String(value || '')
    .split(',')
    .map((model) => model.trim())
    .filter((model) => {
      if (!model || seen.has(model)) return false;
      seen.add(model);
      return true;
    });
};

export const getCommonChannelModels = (channels) => {
  if (!Array.isArray(channels) || channels.length === 0) return [];
  const firstModels = parseChannelModels(channels[0]?.models);
  if (firstModels.length === 0) return [];

  const remainingModelSets = channels.slice(1).map((channel) => {
    return new Set(parseChannelModels(channel?.models));
  });
  if (remainingModelSets.some((models) => models.size === 0)) return [];
  return firstModels.filter((model) =>
    remainingModelSets.every((models) => models.has(model)),
  );
};

const uniqueNumbers = (values) => {
  const seen = new Set();
  return (values || []).map(Number).filter((value) => {
    if (!Number.isInteger(value) || value <= 0 || seen.has(value)) {
      return false;
    }
    seen.add(value);
    return true;
  });
};

const uniqueModels = (values, primaryModel) => {
  const seen = new Set();
  return (values || [])
    .map((value) => String(value || '').trim())
    .filter((value) => {
      if (!value || value === primaryModel || seen.has(value)) return false;
      seen.add(value);
      return true;
    });
};

export const normalizeMonitorGroupPayload = (values, editingGroup) => {
  const primaryModel = String(values.primary_model || '').trim();
  const isEdit = Number(editingGroup?.id) > 0;
  return {
    ...values,
    id: isEdit ? Number(editingGroup.id) : undefined,
    name: String(values.name || '').trim(),
    key: isEdit ? editingGroup.key : String(values.key || '').trim(),
    description: String(values.description || '').trim(),
    primary_model: primaryModel,
    extra_models: uniqueModels(values.extra_models, primaryModel),
    channel_ids: uniqueNumbers(values.channel_ids),
  };
};
