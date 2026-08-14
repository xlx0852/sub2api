import { controlPlaneRequest } from '@/api/generated/request'

export interface PublicVersion {
  version: string
  min_frontend_version: string
}

export async function getPublicVersion(): Promise<PublicVersion> {
  const { data } = await controlPlaneRequest<
    '/api/v1/public/version',
    'get',
    PublicVersion
  >('get', '/api/v1/public/version')
  return data
}
