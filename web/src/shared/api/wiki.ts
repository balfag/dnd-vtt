import { ApiError, apiRequest } from './client'

// Types of docs/architecture/protocol.md, "Вики (M3)"; times are unix milliseconds.

/** PageNode is a row of the page tree. */
export interface PageNode {
  id: string
  parentId: string | null
  kind: 'page' | 'database'
  title: string
  icon: string
  position: number
  /** recordCount is the number of records of a database; 0 for a page. */
  recordCount: number
}

export interface PageRef {
  id: string
  title: string
  icon: string
  kind: 'page' | 'database'
}

export interface RefInfo {
  label: string
  exists: boolean
}

export interface WikiPage {
  id: string
  campaignId: string
  parentId: string | null
  kind: 'page' | 'database'
  isRecord: boolean
  title: string
  icon: string
  coverAssetId: string | null
  content: unknown
  props: Record<string, unknown>
  tags: { id: string; name: string }[]
  path: PageRef[]
  refs: Record<string, RefInfo>
  version: number
  createdAt: number
  updatedAt: number
}

export interface SavePage {
  version: number
  title: string
  icon: string
  coverAssetId: string | null
  content: unknown
  force?: boolean
}

export interface Saved {
  version: number
  updatedAt: number
  refs: Record<string, RefInfo>
}

export interface MentionHit {
  kind: 'page' | 'scene' | 'token'
  id: string
  label: string
  detail: string
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isPageKind(v: unknown): v is 'page' | 'database' {
  return v === 'page' || v === 'database'
}

export function isPageNode(v: unknown): v is PageNode {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    (v.parentId === null || typeof v.parentId === 'string') &&
    isPageKind(v.kind) &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    typeof v.position === 'number' &&
    typeof v.recordCount === 'number'
  )
}

export function isPageRef(v: unknown): v is PageRef {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    isPageKind(v.kind)
  )
}

function isRefInfo(v: unknown): v is RefInfo {
  return isObject(v) && typeof v.label === 'string' && typeof v.exists === 'boolean'
}

function isRefs(v: unknown): v is Record<string, RefInfo> {
  return isObject(v) && Object.values(v).every(isRefInfo)
}

export function isWikiPage(v: unknown): v is WikiPage {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.campaignId === 'string' &&
    (v.parentId === null || typeof v.parentId === 'string') &&
    isPageKind(v.kind) &&
    typeof v.isRecord === 'boolean' &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    (v.coverAssetId === null || typeof v.coverAssetId === 'string') &&
    'content' in v &&
    isObject(v.props) &&
    Array.isArray(v.tags) &&
    v.tags.every(isTag) &&
    Array.isArray(v.path) &&
    v.path.every(isPageRef) &&
    isRefs(v.refs) &&
    typeof v.version === 'number' &&
    typeof v.createdAt === 'number' &&
    typeof v.updatedAt === 'number'
  )
}

export function isSaved(v: unknown): v is Saved {
  return (
    isObject(v) &&
    typeof v.version === 'number' &&
    typeof v.updatedAt === 'number' &&
    isRefs(v.refs)
  )
}

function isTree(v: unknown): v is { pages: PageNode[] } {
  return isObject(v) && Array.isArray(v.pages) && v.pages.every(isPageNode)
}

function isRefList(v: unknown): v is PageRef[] {
  return Array.isArray(v) && v.every(isPageRef)
}

function isMentionHit(v: unknown): v is MentionHit {
  return (
    isObject(v) &&
    (v.kind === 'page' || v.kind === 'scene' || v.kind === 'token') &&
    typeof v.id === 'string' &&
    typeof v.label === 'string' &&
    typeof v.detail === 'string'
  )
}

function isMentionList(v: unknown): v is MentionHit[] {
  return Array.isArray(v) && v.every(isMentionHit)
}

const enc = encodeURIComponent

function need<T>(v: T | undefined): T {
  if (v === undefined) throw new ApiError(200, 'unknown')
  return v
}

/** getWikiTree returns every page of the campaign outside the trash, except database records. */
export async function getWikiTree(campaignId: string): Promise<PageNode[]> {
  const tree = need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/tree`, { guard: isTree }),
  )
  return tree.pages
}

/** createWikiPage makes a page; it becomes the last among its siblings. */
export async function createWikiPage(
  campaignId: string,
  p: {
    parentId?: string | null
    title?: string
    icon?: string
    content?: unknown
    /** templateId is a template of the campaign; then content and icon must be absent. */
    templateId?: string
  },
): Promise<WikiPage> {
  return need(
    await apiRequest('POST', `/api/campaigns/${enc(campaignId)}/wiki/pages`, {
      body: p,
      guard: isWikiPage,
    }),
  )
}

export async function getWikiPage(id: string): Promise<WikiPage> {
  return need(await apiRequest('GET', `/api/wiki/pages/${enc(id)}`, { guard: isWikiPage }))
}

/** saveWikiPage replaces the page; a stale version is an ApiError with code "conflict" (409). */
export async function saveWikiPage(
  id: string,
  p: SavePage,
  opts?: { keepalive?: boolean },
): Promise<Saved> {
  return need(
    await apiRequest('PUT', `/api/wiki/pages/${enc(id)}`, {
      body: p,
      guard: isSaved,
      keepalive: opts?.keepalive,
    }),
  )
}

/** moveWikiPage puts the page under parentId (null: the root) at the place index among the new siblings. */
export async function moveWikiPage(
  id: string,
  parentId: string | null,
  index: number,
): Promise<void> {
  await apiRequest('POST', `/api/wiki/pages/${enc(id)}/move`, { body: { parentId, index } })
}

/** deleteWikiPage sends the page with its subtree to the trash. */
export async function deleteWikiPage(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/pages/${enc(id)}`)
}

/** searchMentions finds pages, scenes and tokens for the "@" menu. */
export async function searchMentions(campaignId: string, q: string): Promise<MentionHit[]> {
  return need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/mention-search?q=${enc(q)}`, {
      guard: isMentionList,
    }),
  )
}

export async function getBacklinks(id: string): Promise<PageRef[]> {
  return need(await apiRequest('GET', `/api/wiki/pages/${enc(id)}/backlinks`, { guard: isRefList }))
}

export interface Tag {
  id: string
  name: string
}

export interface TagCount extends Tag {
  pageCount: number
}

/** SnippetPart is a piece of the text around a search match; match marks the found words. */
export interface SnippetPart {
  text: string
  match?: boolean
}

export interface SearchHit {
  id: string
  title: string
  icon: string
  kind: 'page' | 'database'
  isRecord: boolean
  /** path holds the titles of the ancestors. */
  path: string[]
  snippet: SnippetPart[]
}

export interface TrashItem {
  id: string
  title: string
  icon: string
  kind: 'page' | 'database'
  deletedAt: number
  purgeAt: number
  pageCount: number
}

export interface Template {
  id: string
  name: string
  icon: string
  createdAt: number
}

export function isTag(v: unknown): v is Tag {
  return isObject(v) && typeof v.id === 'string' && typeof v.name === 'string'
}

export function isTagCount(v: unknown): v is TagCount {
  return isTag(v) && 'pageCount' in v && typeof v.pageCount === 'number'
}

function isSnippetPart(v: unknown): v is SnippetPart {
  return (
    isObject(v) &&
    typeof v.text === 'string' &&
    (v.match === undefined || typeof v.match === 'boolean')
  )
}

export function isSearchHit(v: unknown): v is SearchHit {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    isPageKind(v.kind) &&
    typeof v.isRecord === 'boolean' &&
    Array.isArray(v.path) &&
    v.path.every((p) => typeof p === 'string') &&
    Array.isArray(v.snippet) &&
    v.snippet.every(isSnippetPart)
  )
}

export function isTrashItem(v: unknown): v is TrashItem {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    isPageKind(v.kind) &&
    typeof v.deletedAt === 'number' &&
    typeof v.purgeAt === 'number' &&
    typeof v.pageCount === 'number'
  )
}

export function isTemplate(v: unknown): v is Template {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    typeof v.icon === 'string' &&
    typeof v.createdAt === 'number'
  )
}

function listOf<T>(item: (v: unknown) => v is T): (v: unknown) => v is T[] {
  return (v): v is T[] => Array.isArray(v) && v.every(item)
}

function isTags(v: unknown): v is { tags: Tag[] } {
  return isObject(v) && Array.isArray(v.tags) && v.tags.every(isTag)
}

/** setPageTags replaces the tags of a page; tags that do not exist yet are created. */
export async function setPageTags(id: string, names: string[]): Promise<Tag[]> {
  const res = need(
    await apiRequest('PUT', `/api/wiki/pages/${enc(id)}/tags`, {
      body: { names },
      guard: isTags,
    }),
  )
  return res.tags
}

export async function listTags(campaignId: string): Promise<TagCount[]> {
  return need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/tags`, {
      guard: listOf(isTagCount),
    }),
  )
}

/** renameTag renames a tag; a name taken by another tag is a 409, unless merge joins the two. */
export async function renameTag(id: string, name: string, merge?: boolean): Promise<Tag> {
  return need(
    await apiRequest('PATCH', `/api/wiki/tags/${enc(id)}`, {
      body: merge === undefined ? { name } : { name, merge },
      guard: isTag,
    }),
  )
}

export async function deleteTag(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/tags/${enc(id)}`)
}

/** searchWiki finds pages by words (q) and/or by a tag (tag is an id). */
export async function searchWiki(
  campaignId: string,
  p: { q: string; tag?: string; limit?: number },
): Promise<SearchHit[]> {
  const params = [`q=${enc(p.q)}`]
  if (p.tag !== undefined) params.push(`tag=${enc(p.tag)}`)
  if (p.limit !== undefined) params.push(`limit=${String(p.limit)}`)
  return need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/search?${params.join('&')}`, {
      guard: listOf(isSearchHit),
    }),
  )
}

export async function listTrash(campaignId: string): Promise<TrashItem[]> {
  return need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/trash`, {
      guard: listOf(isTrashItem),
    }),
  )
}

export async function restoreTrash(id: string): Promise<void> {
  await apiRequest('POST', `/api/wiki/trash/${enc(id)}/restore`)
}

/** purgeTrash deletes one trash item for good. */
export async function purgeTrash(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/trash/${enc(id)}`)
}

export async function emptyTrash(campaignId: string): Promise<void> {
  await apiRequest('DELETE', `/api/campaigns/${enc(campaignId)}/wiki/trash`)
}

export async function listTemplates(campaignId: string): Promise<Template[]> {
  return need(
    await apiRequest('GET', `/api/campaigns/${enc(campaignId)}/wiki/templates`, {
      guard: listOf(isTemplate),
    }),
  )
}

/** saveTemplate makes a template from a page; more than 100 templates is a 409. */
export async function saveTemplate(
  campaignId: string,
  fromPageId: string,
  name: string,
): Promise<Template> {
  return need(
    await apiRequest('POST', `/api/campaigns/${enc(campaignId)}/wiki/templates`, {
      body: { fromPageId, name },
      guard: isTemplate,
    }),
  )
}

export async function renameTemplate(id: string, name: string): Promise<Template> {
  return need(
    await apiRequest('PATCH', `/api/wiki/templates/${enc(id)}`, {
      body: { name },
      guard: isTemplate,
    }),
  )
}

export async function deleteTemplate(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/templates/${enc(id)}`)
}

/** pageExportUrl is the address of the Markdown file of a page; the browser downloads it with its cookie. */
export function pageExportUrl(id: string): string {
  return `/api/wiki/pages/${enc(id)}/export.md`
}

/** wikiExportUrl is the address of the zip of the whole wiki. */
export function wikiExportUrl(campaignId: string): string {
  return `/api/campaigns/${enc(campaignId)}/wiki/export.zip`
}
