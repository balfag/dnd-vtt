/**
 * debounceSearch makes a search that waits for a pause in the calls: every call within `ms` of
 * the next one gets the answer to the last query, so a menu never flashes an empty list.
 */
export function debounceSearch<T>(
  search: (query: string) => Promise<T[]>,
  ms: number,
): (query: string) => Promise<T[]> {
  let timer: ReturnType<typeof setTimeout> | undefined
  let waiting: { resolve: (v: T[]) => void; reject: (e: unknown) => void }[] = []
  return (query) =>
    new Promise<T[]>((resolve, reject) => {
      waiting.push({ resolve, reject })
      clearTimeout(timer)
      timer = setTimeout(() => {
        const batch = waiting
        waiting = []
        search(query).then(
          (items) => {
            for (const w of batch) w.resolve(items)
          },
          (err: unknown) => {
            for (const w of batch) w.reject(err)
          },
        )
      }, ms)
    })
}
