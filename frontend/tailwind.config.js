/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - 墨色灰阶。业务状态色仍保留，用于成功、警告和错误反馈。
        primary: {
          50: '#f7f7f5',
          100: '#ebebe7',
          200: '#d4d4ce',
          300: '#b4b4ac',
          400: '#8b8b82',
          500: '#65655e',
          600: '#484844',
          700: '#343431',
          800: '#242422',
          900: '#181817',
          950: '#0b0b0a'
        },
        // 辅助色 - 深蓝灰
        accent: {
          50: '#fafaf8',
          100: '#f2f2ef',
          200: '#e3e3de',
          300: '#cdcdc6',
          400: '#a8a8a0',
          500: '#7b7b73',
          600: '#5b5b55',
          700: '#41413d',
          800: '#292927',
          900: '#171716',
          950: '#090909'
        },
        // 深色模式背景
        dark: {
          50: '#f7f7f5',
          100: '#e8e8e4',
          200: '#d0d0ca',
          300: '#abab9f',
          400: '#85857c',
          500: '#62625c',
          600: '#484844',
          700: '#30302e',
          800: '#20201f',
          900: '#141413',
          950: '#090909'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Avenir Next',
          'Noto Sans SC',
          'Source Han Sans SC',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: 'none',
        'glass-sm': 'none',
        glow: '0 10px 30px rgba(12, 12, 11, 0.10)',
        'glow-lg': '0 16px 48px rgba(12, 12, 11, 0.14)',
        card: 'none',
        'card-hover': 'none',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #343431 0%, #0b0b0a 100%)',
        'gradient-dark': 'linear-gradient(135deg, #242422 0%, #090909 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 18% 8%, rgba(28, 28, 26, 0.045) 0px, transparent 32%), radial-gradient(at 88% 90%, rgba(28, 28, 26, 0.035) 0px, transparent 30%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 8px 24px rgba(12, 12, 11, 0.08)' },
          '100%': { boxShadow: '0 12px 34px rgba(12, 12, 11, 0.14)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
