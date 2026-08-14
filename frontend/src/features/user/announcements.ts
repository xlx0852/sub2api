/**
 * User Announcements API endpoints
 */

import { controlPlaneRequest } from '@/api/generated/request'
import type { UserAnnouncement } from '@/types'

export async function list(unreadOnly: boolean = false): Promise<UserAnnouncement[]> {
  const { data } = await controlPlaneRequest<
    '/api/v1/announcements',
    'get',
    UserAnnouncement[]
  >('get', '/api/v1/announcements', {
    query: unreadOnly ? { unread_only: 1 } : {}
  })
  return data
}

export async function markRead(id: number): Promise<{ message: string }> {
  const { data } = await controlPlaneRequest<
    '/api/v1/announcements/{id}/read',
    'post',
    { message: string }
  >('post', '/api/v1/announcements/{id}/read', { pathParams: { id } })
  return data
}

const announcementsAPI = {
  list,
  markRead
}

export default announcementsAPI

