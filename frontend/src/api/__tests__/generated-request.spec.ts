import { describe, expect, it } from 'vitest'
import { CONTROL_PLANE_PREFIX, toClientPath } from '@/api/generated/request'

describe('control-plane request helper', () => {
  it('strips the /api/v1 prefix used by axios baseURL', () => {
    expect(toClientPath(`${CONTROL_PLANE_PREFIX}/auth/login`)).toBe('/auth/login')
    expect(toClientPath(CONTROL_PLANE_PREFIX)).toBe('/')
  })

  it('interpolates OpenAPI path parameters', () => {
    expect(
      toClientPath(`${CONTROL_PLANE_PREFIX}/payment/orders/{id}`, { id: 42 })
    ).toBe('/payment/orders/42')
    expect(
      toClientPath(`${CONTROL_PLANE_PREFIX}/user/account-bindings/{provider}`, {
        provider: 'linuxdo'
      })
    ).toBe('/user/account-bindings/linuxdo')
  })

  it('rejects missing path parameters', () => {
    expect(() => toClientPath(`${CONTROL_PLANE_PREFIX}/keys/{id}`, {})).toThrow(
      /missing OpenAPI path parameter: id/
    )
  })
})
