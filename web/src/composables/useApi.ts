import type { CreateSharePayload, Share, ShareCreatedResult, Site, SiteUpdatePayload } from '../types'

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
  const listSites = async () => {
    const payload = await request<{ sites: Site[] }>('/api/sites')
    return payload.sites
  }

  const createSite = async (input: { name: string; url: string; groupName: string; icon: string }) => {
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
        icon: input.icon,
      }),
    })
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
      }),
    })
  }

  const stopShare = async (shareId: number) => {
    await request<{ status: string }>(`/api/shares/${shareId}/stop`, {
      method: 'POST',
    })
  }

  return {
    listSites,
    createSite,
    updateSite,
    deleteSite,
    incrementSiteClick,
    listShares,
    createShare,
    stopShare,
  }
}
