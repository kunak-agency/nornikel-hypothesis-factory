import { http } from './client'
import type { DocumentListResponse } from './types'

// Эндпоинты базы знаний (документы).
export const documentsApi = {
  list(): Promise<DocumentListResponse> {
    return http.get<DocumentListResponse>('/v1/documents').then((r) => r.data)
  },

  remove(documentId: string): Promise<void> {
    return http.delete(`/v1/documents/${documentId}`).then(() => undefined)
  },
}
