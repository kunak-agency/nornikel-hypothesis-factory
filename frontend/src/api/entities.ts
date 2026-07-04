import { http } from './client'
import type { EntityReputationListResponse } from './types'

// Репутация сущностей — накопленный по фидбэку «мягкий сигнал» для будущих прогонов.
export const entitiesApi = {
  reputation(): Promise<EntityReputationListResponse> {
    return http
      .get<EntityReputationListResponse>('/v1/entities/reputation')
      .then((r) => r.data)
  },
}
