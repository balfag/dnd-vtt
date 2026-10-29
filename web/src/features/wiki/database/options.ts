import { ApiError } from '../../../shared/api/client'
import { updateProperty, type Property } from '../../../shared/api/wiki'
import { nextColor, optionsOf } from './model'

/**
 * addOption adds a variant to a select (the next color of the palette; the server gives it an id)
 * and returns the id; patch gets the property as the server now has it.
 */
export async function addOption(
  prop: Property,
  name: string,
  patch: (saved: Property) => void,
): Promise<string> {
  const list = optionsOf(prop)
  const saved = await updateProperty(prop.id, {
    options: { ...prop.options, options: [...list, { name, color: nextColor(list) }] },
  })
  patch(saved)
  const known = new Set(list.map((o) => o.id))
  const made = optionsOf(saved).find((o) => o.id !== undefined && !known.has(o.id))
  if (made?.id === undefined) throw new ApiError(200, 'unknown')
  return made.id
}
