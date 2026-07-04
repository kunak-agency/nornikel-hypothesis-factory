import { http } from './client'
import type {
  PlantEquipmentListResponse,
  PlantEquipment,
  PlantEquipmentRequest,
  PlantsResponse,
} from './types'

// Справочник оборудования фабрик (полный CRUD) + список фабрик для группировки.
export const equipmentApi = {
  list(plant?: string): Promise<PlantEquipmentListResponse> {
    return http
      .get<PlantEquipmentListResponse>('/v1/plant-equipment', {
        params: plant ? { plant } : undefined,
      })
      .then((r) => r.data)
  },

  create(body: PlantEquipmentRequest): Promise<PlantEquipment> {
    return http.post<PlantEquipment>('/v1/plant-equipment', body).then((r) => r.data)
  },

  update(id: string, body: PlantEquipmentRequest): Promise<PlantEquipment> {
    return http.put<PlantEquipment>(`/v1/plant-equipment/${id}`, body).then((r) => r.data)
  },

  remove(id: string): Promise<void> {
    return http.delete(`/v1/plant-equipment/${id}`).then(() => undefined)
  },

  plants(): Promise<PlantsResponse> {
    return http.get<PlantsResponse>('/v1/plants').then((r) => r.data)
  },
}
