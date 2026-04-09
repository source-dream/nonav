export interface Site {
  id: number
  name: string
  url: string
  groupName: string
  checkEnabled: boolean
  icon: string
  clickCount: number
  createdAt: string
  updatedAt: string
}

export type SiteStatus = 'checking' | 'online' | 'offline' | 'disabled'

export interface SiteStatusResult {
  siteId: number
  status: Exclude<SiteStatus, 'checking'>
  checkedAt: string
}

export interface Share {
  id: number
  siteId: number
  siteName: string
  shareMode: 'path_ctx' | 'subdomain'
  subdomainSlug?: string
  token: string
  status: 'active' | 'stopped' | 'expired'
  expiresAt: string
  createdAt: string
  updatedAt: string
  stoppedAt?: string
  accessHits: number
  shareUrl: string
}

export interface ShareCreatedResult {
  share: {
    id: number
    siteId: number
    siteName: string
    shareMode: 'path_ctx' | 'subdomain'
    subdomainSlug?: string
    token: string
    status: 'active' | 'stopped' | 'expired'
    expiresAt: string
    createdAt: string
    updatedAt: string
  }
  shareUrl: string
  plainPassword: string
}

export interface SiteUpdatePayload {
  id: number
  name: string
  url: string
  groupName: string
  checkEnabled: boolean
}

export interface CreateSharePayload {
  siteId: number
  expiresInHours?: number
  password?: string
  shareMode?: 'path_ctx' | 'subdomain'
  subdomainSlug?: string
}

export type GatewayHealth = 'online' | 'degraded' | 'offline'

export type ServiceStatusKey = 'gateway' | 'nonav' | 'frpc' | 'frps'

export interface ServiceStatusItem {
  key: ServiceStatusKey
  label: string
  health: GatewayHealth
  summary: string
}

export interface GatewayStatusSnapshot {
  health: GatewayHealth
  services: ServiceStatusItem[]
}

export type SystemLogSource = 'nonav' | 'nonav-gateway'

export interface SystemLogEntry {
  id: number
  source: SystemLogSource
  timestamp: string
  event: string
  message: string
  req: string
  tone: 'normal' | 'warning' | 'error'
  details: string[]
}
