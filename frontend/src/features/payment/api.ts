/**
 * User Payment API endpoints
 * Handles payment operations for regular users
 */

import { controlPlaneRequest } from '@/api/generated/request'
import type {
  PaymentConfig,
  MethodLimitsResponse,
  CheckoutInfoResponse,
  CreateOrderRequest,
  CreateOrderResult,
  PaymentOrder
} from '@/types/payment'
import type { BasePaginationResponse } from '@/types'

export interface PublicOrderVerifyResult {
  out_trade_no: string
  status: string
  paid: boolean
  created_at: string
  expires_at: string
}

export const paymentAPI = {
  /** Get payment configuration (enabled types, limits, etc.) */
  getConfig() {
    return controlPlaneRequest<'/api/v1/payment/config', 'get', PaymentConfig>(
      'get',
      '/api/v1/payment/config'
    )
  },

  /** Get all checkout page data in a single call */
  getCheckoutInfo() {
    return controlPlaneRequest<'/api/v1/payment/checkout-info', 'get', CheckoutInfoResponse>(
      'get',
      '/api/v1/payment/checkout-info'
    )
  },

  /** Get payment method limits and fee rates */
  getLimits() {
    return controlPlaneRequest<'/api/v1/payment/limits', 'get', MethodLimitsResponse>(
      'get',
      '/api/v1/payment/limits'
    )
  },

  /** Create a new payment order */
  createOrder(data: CreateOrderRequest) {
    return controlPlaneRequest<'/api/v1/payment/orders', 'post', CreateOrderResult>(
      'post',
      '/api/v1/payment/orders',
      { body: data }
    )
  },

  /** Get current user's orders */
  getMyOrders(params?: { page?: number; page_size?: number; status?: string }) {
    return controlPlaneRequest<
      '/api/v1/payment/orders/my',
      'get',
      BasePaginationResponse<PaymentOrder>
    >('get', '/api/v1/payment/orders/my', { query: params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return controlPlaneRequest<'/api/v1/payment/orders/{id}', 'get', PaymentOrder>(
      'get',
      '/api/v1/payment/orders/{id}',
      { pathParams: { id } }
    )
  },

  /** Cancel a pending order */
  cancelOrder(id: number) {
    return controlPlaneRequest('post', '/api/v1/payment/orders/{id}/cancel', {
      pathParams: { id }
    })
  },

  /** Verify order payment status with upstream provider */
  verifyOrder(outTradeNo: string) {
    return controlPlaneRequest<'/api/v1/payment/orders/verify', 'post', PaymentOrder>(
      'post',
      '/api/v1/payment/orders/verify',
      { body: { out_trade_no: outTradeNo } }
    )
  },

  /** Legacy-compatible public order lookup by out_trade_no */
  verifyOrderPublic(outTradeNo: string) {
    return controlPlaneRequest<
      '/api/v1/payment/public/orders/verify',
      'post',
      PublicOrderVerifyResult
    >('post', '/api/v1/payment/public/orders/verify', {
      body: { out_trade_no: outTradeNo }
    })
  },

  /** Resolve an order from a signed resume token without auth */
  resolveOrderPublicByResumeToken(resumeToken: string) {
    return controlPlaneRequest<
      '/api/v1/payment/public/orders/resolve',
      'post',
      PublicOrderVerifyResult
    >('post', '/api/v1/payment/public/orders/resolve', {
      body: { resume_token: resumeToken }
    })
  },

  /** Request a refund for a completed order */
  requestRefund(id: number, data: { reason: string }) {
    return controlPlaneRequest('post', '/api/v1/payment/orders/{id}/refund-request', {
      pathParams: { id },
      body: data
    })
  },

  /** Get provider instance IDs that allow user refund */
  getRefundEligibleProviders() {
    return controlPlaneRequest<
      '/api/v1/payment/orders/refund-eligible-providers',
      'get',
      { provider_instance_ids: string[] }
    >('get', '/api/v1/payment/orders/refund-eligible-providers')
  }
}
