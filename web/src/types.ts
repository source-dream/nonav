export interface Site {
  id: number
  name: string
  url: string
  groupName: string
  icon: string
  clickCount: number
  createdAt: string
  updatedAt: string
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
}

export interface CreateSharePayload {
  siteId: number
  expiresInHours?: number
  password?: string
  shareMode?: 'path_ctx' | 'subdomain'
  subdomainSlug?: string
}
