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
    kind?: 'page' | 'database'
    /** props are the values of a record, by property id. */
    props?: Record<string, unknown>
    /** database is the first properties and views of a new database (kind database). */
    database?: {
      properties: { name: string; type: PropertyType; options?: PropertyOptions }[]
      views: { name: string; kind: ViewKind }[]
    }
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

export type PropertyType =
  | 'text'
  | 'number'
  | 'select'
  | 'multi_select'
  | 'checkbox'
  | 'date'
  | 'url'
  | 'relation'
  | 'game_link'

export const propertyTypes: readonly PropertyType[] = [
  'text',
  'number',
  'select',
  'multi_select',
  'checkbox',
  'date',
  'url',
  'relation',
  'game_link',
]

export type OptionColor =
  'gray' | 'brown' | 'orange' | 'yellow' | 'green' | 'blue' | 'purple' | 'red'

export const optionColors: readonly OptionColor[] = [
  'gray',
  'brown',
  'orange',
  'yellow',
  'green',
  'blue',
  'purple',
  'red',
]

/** SelectOption is a variant of select and multi_select; a new one is sent without an id. */
export interface SelectOption {
  id?: string
  name: string
  color: OptionColor
}

/** PropertyOptions: select and multi_select have options, number has format, relation has targetDatabaseId. */
export interface PropertyOptions {
  options?: SelectOption[]
  format?: 'integer' | 'decimal'
  targetDatabaseId?: string
}

export interface Property {
  id: string
  name: string
  type: PropertyType
  position: number
  options: PropertyOptions
}

export type ViewKind = 'table' | 'cards'

/** Filter compares a property (propertyId) or a built-in field (field: title, createdAt, updatedAt, tags). */
export interface Filter {
  propertyId?: string
  field?: 'title' | 'createdAt' | 'updatedAt' | 'tags'
  op: string
  value?: string | number
}

export interface SortKey {
  propertyId?: string
  field?: 'title' | 'createdAt' | 'updatedAt'
  dir: 'asc' | 'desc'
}

export interface ViewConfig {
  filters: Filter[]
  sorts: SortKey[]
  visible: string[]
  /** widths are pixels by property id or "title". */
  widths: Record<string, number>
}

export interface View {
  id: string
  name: string
  kind: ViewKind
  position: number
  config: ViewConfig
}

/** DbRecord is a row of a database: a child page with its values (named so to keep the global Record type). */
export interface DbRecord {
  id: string
  title: string
  icon: string
  coverAssetId: string | null
  props: Record<string, unknown>
  tags: Tag[]
  createdAt: number
  updatedAt: number
}

export interface Database {
  properties: Property[]
  views: View[]
  records: DbRecord[]
}

/** GameLinkValue is an item of a game_link value. */
export interface GameLinkValue {
  kind: 'scene' | 'token'
  id: string
}

export function isPropertyType(v: unknown): v is PropertyType {
  return typeof v === 'string' && (propertyTypes as readonly string[]).includes(v)
}

export function isProperty(v: unknown): v is Property {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    isPropertyType(v.type) &&
    typeof v.position === 'number' &&
    isObject(v.options)
  )
}

function isStringList(v: unknown): v is string[] {
  return Array.isArray(v) && v.every((x) => typeof x === 'string')
}

function isFilter(v: unknown): v is Filter {
  return (
    isObject(v) &&
    typeof v.op === 'string' &&
    (v.propertyId === undefined || typeof v.propertyId === 'string') &&
    (v.field === undefined || typeof v.field === 'string') &&
    (v.value === undefined || typeof v.value === 'string' || typeof v.value === 'number')
  )
}

function isSortKey(v: unknown): v is SortKey {
  return (
    isObject(v) &&
    (v.dir === 'asc' || v.dir === 'desc') &&
    (v.propertyId === undefined || typeof v.propertyId === 'string') &&
    (v.field === undefined || typeof v.field === 'string')
  )
}

export function isViewConfig(v: unknown): v is ViewConfig {
  return (
    isObject(v) &&
    Array.isArray(v.filters) &&
    v.filters.every(isFilter) &&
    Array.isArray(v.sorts) &&
    v.sorts.every(isSortKey) &&
    isStringList(v.visible) &&
    isObject(v.widths) &&
    Object.values(v.widths).every((w) => typeof w === 'number')
  )
}

export function isView(v: unknown): v is View {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    (v.kind === 'table' || v.kind === 'cards') &&
    typeof v.position === 'number' &&
    isViewConfig(v.config)
  )
}

export function isDbRecord(v: unknown): v is DbRecord {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.title === 'string' &&
    typeof v.icon === 'string' &&
    (v.coverAssetId === null || typeof v.coverAssetId === 'string') &&
    isObject(v.props) &&
    Array.isArray(v.tags) &&
    v.tags.every(isTag) &&
    typeof v.createdAt === 'number' &&
    typeof v.updatedAt === 'number'
  )
}

export function isDatabase(v: unknown): v is Database {
  return (
    isObject(v) &&
    Array.isArray(v.properties) &&
    v.properties.every(isProperty) &&
    Array.isArray(v.views) &&
    v.views.every(isView) &&
    Array.isArray(v.records) &&
    v.records.every(isDbRecord)
  )
}

function isPropsReply(v: unknown): v is { props: Record<string, unknown> } {
  return isObject(v) && isObject(v.props)
}

export async function getDatabase(pageId: string): Promise<Database> {
  return need(await apiRequest('GET', `/api/wiki/databases/${enc(pageId)}`, { guard: isDatabase }))
}

/** addProperty adds a property as the last one; a name used in the database is a 400. */
export async function addProperty(
  pageId: string,
  p: { name: string; type: PropertyType; options?: PropertyOptions },
): Promise<Property> {
  return need(
    await apiRequest('POST', `/api/wiki/databases/${enc(pageId)}/properties`, {
      body: { name: p.name, type: p.type, options: p.options ?? {} },
      guard: isProperty,
    }),
  )
}

/** updateProperty changes the name, the options or the place (position) of a property; its type never changes. */
export async function updateProperty(
  id: string,
  patch: { name?: string; options?: PropertyOptions; position?: number },
): Promise<Property> {
  return need(
    await apiRequest('PATCH', `/api/wiki/properties/${enc(id)}`, {
      body: patch,
      guard: isProperty,
    }),
  )
}

export async function deleteProperty(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/properties/${enc(id)}`)
}

export async function addView(
  pageId: string,
  v: { name: string; kind: ViewKind; config?: ViewConfig },
): Promise<View> {
  return need(
    await apiRequest('POST', `/api/wiki/databases/${enc(pageId)}/views`, {
      body: v,
      guard: isView,
    }),
  )
}

export async function updateView(
  id: string,
  patch: { name?: string; config?: ViewConfig; position?: number },
): Promise<View> {
  return need(
    await apiRequest('PATCH', `/api/wiki/views/${enc(id)}`, { body: patch, guard: isView }),
  )
}

/** deleteView removes a view; the last view of a database is a 409. */
export async function deleteView(id: string): Promise<void> {
  await apiRequest('DELETE', `/api/wiki/views/${enc(id)}`)
}

/** setRecordProps changes the given values of a record; null removes a value. A bad value is a 400. */
export async function setRecordProps(
  recordId: string,
  props: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const res = need(
    await apiRequest('PATCH', `/api/wiki/pages/${enc(recordId)}/props`, {
      body: { props },
      guard: isPropsReply,
    }),
  )
  return res.props
}
