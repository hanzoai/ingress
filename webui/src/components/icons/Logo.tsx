import { CSSProperties } from 'react'

import { MARK_BLOCKS, MARK_SHADE } from '@hanzo/logo/logos'

import { useIsDarkMode } from 'hooks/use-theme'

type LogoProps = {
  width?: number
  height?: number
  style?: CSSProperties
  isSmallScreen?: boolean
}

const HanzoMark = ({ fill = '#ffffff', ...props }) => (
  <svg viewBox="0 0 67 67" xmlns="http://www.w3.org/2000/svg" {...props}>
    {MARK_BLOCKS.map((d) => (
      <path key={d} d={d} fill={fill} />
    ))}
    {MARK_SHADE.map((d) => (
      <path key={d} d={d} fill={fill} opacity="0.7" />
    ))}
  </svg>
)

const Logo = ({ isSmallScreen, ...props }: LogoProps) => {
  const isDarkMode = useIsDarkMode()
  const fill = isDarkMode ? '#ffffff' : '#000000'

  if (isSmallScreen) {
    return <HanzoMark fill={fill} width={36} {...props} />
  }

  return <HanzoMark fill={fill} {...props} />
}

export default Logo
