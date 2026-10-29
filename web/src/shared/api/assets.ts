import { ApiError, apiRequest, errorCodeOf } from './client'

/** Asset is an uploaded file of a campaign (docs/architecture/protocol.md, "Ассеты"). */
export interface Asset {
  id: string
  campaignId: string
  kind: string
  mime: string
  size: number
  width: number
  height: number
  originalName: string
  createdAt: number
  url: string
}

export function isAsset(v: unknown): v is Asset {
  return (
    typeof v === 'object' &&
    v !== null &&
    !Array.isArray(v) &&
    'id' in v &&
    typeof v.id === 'string' &&
    'campaignId' in v &&
    typeof v.campaignId === 'string' &&
    'kind' in v &&
    typeof v.kind === 'string' &&
    'mime' in v &&
    typeof v.mime === 'string' &&
    'size' in v &&
    typeof v.size === 'number' &&
    'width' in v &&
    typeof v.width === 'number' &&
    'height' in v &&
    typeof v.height === 'number' &&
    'originalName' in v &&
    typeof v.originalName === 'string' &&
    'createdAt' in v &&
    typeof v.createdAt === 'number' &&
    'url' in v &&
    typeof v.url === 'string'
  )
}

function isAssetList(v: unknown): v is Asset[] {
  return Array.isArray(v) && v.every(isAsset)
}

const enc = encodeURIComponent

/** listAssets returns the assets of a campaign, the newest first. */
export async function listAssets(campaignId: string): Promise<Asset[]> {
  const list = await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/assets`, {
    guard: isAssetList,
  })
  if (list === undefined) throw new ApiError(200, 'unknown')
  return list
}

function parseBody(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

/** failureCode names a failed upload: the code of the body, or one told by the status. */
function failureCode(status: number, body: unknown): string {
  const code = errorCodeOf(body)
  if (code !== 'unknown') return code
  if (status === 413) return 'too_large'
  if (status === 415) return 'unsupported_media_type'
  return 'unknown'
}

/**
 * uploadAsset posts multipart (kind before file: the server reads the parts in order)
 * with XMLHttpRequest, because fetch cannot report upload progress. Errors are ApiError.
 */
export function uploadAsset(
  campaignId: string,
  file: File,
  kind: 'map' | 'token' | 'other',
  onProgress?: (p: number) => void,
): Promise<Asset> {
  return new Promise<Asset>((resolve, reject) => {
    const form = new FormData()
    form.append('kind', kind)
    form.append('file', file)
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `/api/campaigns/${enc(campaignId)}/assets`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && e.total > 0) onProgress?.(e.loaded / e.total)
    }
    xhr.onload = () => {
      const body = parseBody(xhr.responseText)
      if (xhr.status >= 200 && xhr.status < 300) {
        if (isAsset(body)) resolve(body)
        else reject(new ApiError(xhr.status, 'unknown'))
        return
      }
      reject(new ApiError(xhr.status, failureCode(xhr.status, body)))
    }
    xhr.onerror = () => {
      reject(new ApiError(0, 'network'))
    }
    xhr.onabort = () => {
      reject(new ApiError(0, 'network'))
    }
    xhr.send(form)
  })
}
