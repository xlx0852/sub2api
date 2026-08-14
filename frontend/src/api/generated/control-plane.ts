/**
 * Generated from openapi/control-plane.yaml. Do not edit.
 * Regenerate with: npm run generate:api
 */
export interface paths {
  '/api/v1/announcements': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/announcements/{id}/read': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/forgot-password': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/login/2fa': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/logout': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/me': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/bind-token': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/bind-login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/bind/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/create-account': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/dingtalk/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/github/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/github/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/github/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/google/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/google/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/google/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/bind-login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/bind/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/create-account': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/linuxdo/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/bind-login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/bind/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/create-account': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/oidc/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/pending/bind-login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/pending/create-account': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/pending/exchange': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/pending/send-verify-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/bind-login': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/bind/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/complete-registration': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/create-account': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/payment/callback': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/payment/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/oauth/wechat/start': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/refresh': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/register': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/reset-password': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/revoke-all-sessions': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/send-verify-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/validate-invitation-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/auth/validate-promo-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/channels/available': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/groups/available': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/groups/rates': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/keys': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/keys/{id}': {
    delete: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
    put: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/model-catalog': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/checkout-info': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/config': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/limits': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/my': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/refund-eligible-providers': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/verify': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/{id}': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/{id}/cancel': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/orders/{id}/refund-request': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/public/orders/resolve': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/public/orders/verify': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/webhook/airwallex': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/webhook/alipay': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/webhook/easypay': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/webhook/stripe': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/payment/webhook/wxpay': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/public/config': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/public/version': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/public/pricing': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/redeem': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/redeem/history': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/settings/email-unsubscribe': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/settings/public': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/api-keys-usage': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/availability': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/group-availability': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/models': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/snapshot-v2': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/stats': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/dashboard/trend': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/errors': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/errors/{id}': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/stats': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/usage/{id}': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user': {
    put: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/account-bindings/email': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/account-bindings/email/send-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/account-bindings/{provider}': {
    delete: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/aff': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/aff/transfer': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/api-keys/{id}/usage/daily': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/auth-identities/bind/start': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/notify-email': {
    delete: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/notify-email/send-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/notify-email/toggle': {
    put: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/notify-email/verify': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/password': {
    put: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/platform-quotas': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/profile': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/disable': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/enable': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/send-code': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/setup': {
    post: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/status': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
  '/api/v1/user/totp/verification-method': {
    get: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }
  }
}

export interface components {
  schemas: Record<string, unknown>
}

export interface operations {
  [operationId: string]: unknown
}

export interface webhooks {}
