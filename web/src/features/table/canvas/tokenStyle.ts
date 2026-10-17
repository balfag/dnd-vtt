import type { PlayerInfo, Token } from '../../../shared/protocol'

export interface TokenStyle {
  fill: string // '#2E3D49' owned, '#4a2a2f' without owners
  stroke: string // first owner's colour; '#ECE7DB' without owners; '#C9A34E' on layer gm
  strokeWidth: number // 4 owned, 2 otherwise
  dashed: boolean // hidden or layer gm (seen only by the GM)
  alpha: number // 0.6 when dashed, else 1
  showName: 'always' | 'hover' | 'never'
  letter: string // first letter of name, upper case; '' when name is ''
}

const ownedFill = '#2E3D49'
const ownerlessFill = '#4a2a2f'
const ownerlessStroke = '#ECE7DB'
const gmLayerStroke = '#C9A34E'

/** tokenStyle says how to draw a token for the viewer. */
export function tokenStyle(t: Token, players: PlayerInfo[], isGM: boolean): TokenStyle {
  const owned = t.owners.length > 0
  const dashed = t.hidden || t.layer === 'gm'
  const ownerColor = players.find((p) => p.id === t.owners[0])?.color
  let showName: TokenStyle['showName'] = 'never'
  if (t.name !== '') {
    if (t.nameVisibility === 'all') showName = 'always'
    else if (t.nameVisibility === 'hover') showName = 'hover'
    else showName = isGM ? 'always' : 'never'
  }
  return {
    fill: owned ? ownedFill : ownerlessFill,
    stroke: t.layer === 'gm' ? gmLayerStroke : (ownerColor ?? ownerlessStroke),
    strokeWidth: owned ? 4 : 2,
    dashed,
    alpha: dashed ? 0.6 : 1,
    showName,
    letter: t.name === '' ? '' : (Array.from(t.name)[0] ?? '').toUpperCase(),
  }
}
