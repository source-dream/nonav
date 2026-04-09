import type { CreateSharePayload, GatewayStatusSnapshot, Share, ShareCreatedResult, Site, SiteStatusResult, SiteUpdatePayload, SystemLogEntry } from '../types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    let message = '请求失败'
    try {
      const payload = (await response.json()) as { error?: string }
      message = payload.error ?? message
    } catch {
      // Ignore JSON parse error and keep fallback message.
    }
    throw new Error(message)
  }

  if (response.status === 204) {
    return {} as T
  }

  return (await response.json()) as T
}

export function useApi() {
  const getSystemStatus = async () => {
    const payload = await request<{
      overallHealth: GatewayStatusSnapshot['health']
      services: GatewayStatusSnapshot['services']
    }>('/api/system/status')

    return {
      health: payload.overallHealth,
      services: payload.services,
    } satisfies GatewayStatusSnapshot
  }

  const getSystemLogs = async () => {
    const payload = await request<{
      logs: Array<{
        id: number
        source: SystemLogEntry['source']
        level: 'info' | 'warn' | 'error'
        timestamp: string
        event: string
        message: string
        req: string
        details: string[]
      }>
    }>('/api/system/logs?limit=200')

    return payload.logs.map((item) => ({
      id: item.id,
      source: item.source,
      timestamp: item.timestamp,
      event: item.event,
      message: item.message,
      req: item.req,
      tone: item.level === 'error' ? 'error' : item.level === 'warn' ? 'warning' : 'normal',
      details: item.details ?? [],
    })) satisfies SystemLogEntry[]
  }

  const listSites = async () => {
    const payload = await request<{ sites: Site[] }>('/api/sites')
    return payload.sites
  }

  const createSite = async (input: { name: string; url: string; groupName: string }) => {
    return request<Site>('/api/sites', {
      method: 'POST',
      body: JSON.stringify(input),
    })
  }

  const deleteSite = async (siteId: number) => {
    await request<{ status: string }>(`/api/sites/${siteId}`, {
      method: 'DELETE',
    })
  }

  const updateSite = async (input: SiteUpdatePayload) => {
    return request<Site>(`/api/sites/${input.id}`, {
      method: 'PUT',
      body: JSON.stringify({
        name: input.name,
        url: input.url,
        groupName: input.groupName,
        checkEnabled: input.checkEnabled,
        icon: '',
      }),
    })
  }

  const checkSiteStatuses = async (siteIds: number[]) => {
    const payload = await request<{ statuses: SiteStatusResult[] }>('/api/site-statuses', {
      method: 'POST',
      body: JSON.stringify({ siteIds }),
    })
    return payload.statuses
  }

  const incrementSiteClick = async (siteId: number) => {
    await request<{ status: string }>(`/api/sites/${siteId}/click`, {
      method: 'POST',
    })
  }

  const listShares = async () => {
    const payload = await request<{ shares: Share[] }>('/api/shares')
    return payload.shares
  }

  const createShare = async (input: CreateSharePayload) => {
    return request<ShareCreatedResult>('/api/shares', {
      method: 'POST',
      body: JSON.stringify({
        siteId: input.siteId,
        expiresInHours: input.expiresInHours,
        password: input.password,
        shareMode: input.shareMode,
        subdomainSlug: input.subdomainSlug,
      }),
    })
  }

  const stopShare = async (shareId: number) => {
    await request<{ status: string }>(`/api/shares/${shareId}/stop`, {
      method: 'POST',
    })
  }

  return {
    getSystemStatus,
    getSystemLogs,
    listSites,
    createSite,
    updateSite,
    checkSiteStatuses,
    deleteSite,
    incrementSiteClick,
    listShares,
    createShare,
    stopShare,
  }
}
