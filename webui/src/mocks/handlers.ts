import { http, passthrough } from 'msw'

import apiEntrypoints from './data/api-entrypoints.json'
import apiHttpMiddlewares from './data/api-http_middlewares.json'
import apiHttpRouters from './data/api-http_routers.json'
import apiHttpServices from './data/api-http_services.json'
import apiOverview from './data/api-overview.json'
import apiTcpMiddlewares from './data/api-tcp_middlewares.json'
import apiTcpRouters from './data/api-tcp_routers.json'
import apiTcpServices from './data/api-tcp_services.json'
import apiUdpRouters from './data/api-udp_routers.json'
import apiUdpServices from './data/api-udp_services.json'
import apiVersion from './data/api-version.json'
import eeApiErrors from './data/ee-api-errors.json'
import { listHandlers } from './utils'

export const getHandlers = (noDelay: boolean = false) => [
  ...listHandlers('/v1/ingress/entrypoints', apiEntrypoints, noDelay, true),
  ...listHandlers('/v1/ingress/errors', eeApiErrors, noDelay),
  ...listHandlers('/v1/ingress/http/middlewares', apiHttpMiddlewares, noDelay),
  ...listHandlers('/v1/ingress/http/routers', apiHttpRouters, noDelay),
  ...listHandlers('/v1/ingress/http/services', apiHttpServices, noDelay),
  ...listHandlers('/v1/ingress/overview', apiOverview, noDelay),
  ...listHandlers('/v1/ingress/tcp/middlewares', apiTcpMiddlewares, noDelay),
  ...listHandlers('/v1/ingress/tcp/routers', apiTcpRouters, noDelay),
  ...listHandlers('/v1/ingress/tcp/services', apiTcpServices, noDelay),
  ...listHandlers('/v1/ingress/udp/routers', apiUdpRouters, noDelay),
  ...listHandlers('/v1/ingress/udp/services', apiUdpServices, noDelay),
  ...listHandlers('/v1/ingress/version', apiVersion, noDelay),
  http.get('*.tsx', () => passthrough()),
  http.get('/img/*', () => passthrough()),
]
