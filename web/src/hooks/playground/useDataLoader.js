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

import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  processModelsData,
  processGroupsData,
  showError,
} from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
) => {
  const { t } = useTranslation();
  const modelRequestRef = useRef(0);
  const inputsRef = useRef(inputs);
  const userStateRef = useRef(userState);

  inputsRef.current = inputs;
  userStateRef.current = userState;

  const loadModels = useCallback(async () => {
    const requestedGroup = inputs.group;
    const requestId = ++modelRequestRef.current;
    try {
      const res = await API.get(API_ENDPOINTS.USER_MODELS, {
        params: {
          include_mapping: true,
          ...(requestedGroup ? { group: requestedGroup } : {}),
        },
      });
      const { success, message, data } = res.data;

      if (requestId !== modelRequestRef.current) {
        return;
      }

      if (success) {
        const { modelOptions, selectedModel } = processModelsData(
          data,
          inputsRef.current.model,
        );
        setModels(modelOptions);

        if (selectedModel !== inputsRef.current.model) {
          // A group switch should keep the playground usable by selecting its
          // first model instead of retaining an unroutable model.
          handleInputChange('model', selectedModel || '');
        }
      } else {
        if (message) {
          showError(t(message));
        }
      }
    } catch (error) {
      if (requestId === modelRequestRef.current) {
        showError(t('加载模型失败'));
      }
    }
  }, [inputs.group, handleInputChange, setModels, t]);

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS);
      const { success, message, data } = res.data;

      if (success) {
        const userGroup =
          userStateRef.current?.user?.group ||
          JSON.parse(localStorage.getItem('user'))?.group;
        const groupOptions = processGroupsData(data, userGroup);
        setGroups(groupOptions);

        const hasCurrentGroup = groupOptions.some(
          (option) => option.value === inputsRef.current.group,
        );
        if (!hasCurrentGroup) {
          handleInputChange('group', groupOptions[0]?.value || '');
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载分组失败'));
    }
  }, [handleInputChange, setGroups, t]);

  // 自动加载数据
  useEffect(() => {
    if (userState?.user) {
      loadGroups();
    }
  }, [userState?.user, loadGroups]);

  useEffect(() => {
    if (userState?.user && inputs.group) {
      loadModels();
    }
  }, [userState?.user, inputs.group, loadModels]);

  return {
    loadModels,
    loadGroups,
  };
};
