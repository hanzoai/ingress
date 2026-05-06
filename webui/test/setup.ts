// Match the production API mount point. `BASE_PATH` (from
// src/libs/utils.ts) reads window.APIUrl on first import; without
// this, fetches resolve to '/http/routers/<x>' instead of
// '/v1/ingress/http/routers/<x>' and miss every msw handler.
//
// Must run BEFORE the imports below — Vitest's setupFiles are
// processed in source order and the mock server captures BASE_PATH
// at module load.
;(globalThis as unknown as { window: { APIUrl: string } }).window =
  (globalThis as unknown as { window?: { APIUrl?: string } }).window || ({} as { APIUrl: string })
;(globalThis as unknown as { window: { APIUrl: string } }).window.APIUrl = '/v1/ingress'

import '@testing-library/jest-dom'
import 'vitest-canvas-mock'
import '@vitest/web-worker'

import * as matchers from 'jest-extended'
import { expect } from 'vitest'

import { server } from '../src/mocks/server'

expect.extend(matchers)

export class IntersectionObserver {
  root = null
  rootMargin = ''
  thresholds = []
  scrollMargin = ''

  disconnect() {
    return null
  }

  observe() {
    return null
  }

  takeRecords() {
    return []
  }

  unobserve() {
    return null
  }
}

class ResizeObserver {
  observe() {
    return null
  }
  unobserve() {
    return null
  }
  disconnect() {
    return null
  }
}

beforeAll(() => {
  globalThis.IntersectionObserver = IntersectionObserver
  window.IntersectionObserver = IntersectionObserver

  globalThis.ResizeObserver = ResizeObserver
  window.ResizeObserver = ResizeObserver

  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(), // deprecated
      removeListener: vi.fn(), // deprecated
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  Object.defineProperty(window, 'scrollTo', {
    writable: true,
    value: vi.fn(),
  })

  server.listen({ onUnhandledRequest: 'error' })
})

afterEach(() => server.resetHandlers())

afterAll(() => server.close())
