import React, { useState } from 'react'
import styles from './Avatar.module.css'

type AvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
type AvatarStatus = 'online' | 'offline' | 'away' | 'busy'

interface AvatarProps {
  src?: string
  alt?: string
  initials?: string
  size?: AvatarSize
  color?: string
  status?: AvatarStatus
  className?: string
}

const PALETTE = [
  '#6366F1', '#8B5CF6', '#EC4899', '#F59E0B',
  '#10B981', '#3B82F6', '#EF4444', '#14B8A6',
]

function hashColor(text: string): string {
  let n = 0
  for (let i = 0; i < text.length; i++) n = (n + text.charCodeAt(i)) % PALETTE.length
  return PALETTE[n]
}

function SilhouetteIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className={styles.silhouette}>
      <path d="M12 12c2.7 0 4.8-2.1 4.8-4.8S14.7 2.4 12 2.4 7.2 4.5 7.2 7.2 9.3 12 12 12zm0 2.4c-3.2 0-9.6 1.6-9.6 4.8v2.4h19.2v-2.4c0-3.2-6.4-4.8-9.6-4.8z" />
    </svg>
  )
}

export function Avatar({
  src,
  alt = '',
  initials,
  size = 'md',
  color,
  status,
  className,
}: AvatarProps) {
  const [imgError, setImgError] = useState(false)
  const bgColor = color ?? (initials ? hashColor(initials) : '#9CA3AF')
  const showImg = src && !imgError

  return (
    <span
      className={[styles.avatar, styles[`size-${size}`], className].filter(Boolean).join(' ')}
      style={!showImg ? { background: bgColor } : undefined}
      role="img"
      aria-label={alt || initials || 'Avatar'}
    >
      {showImg ? (
        <img
          src={src}
          alt={alt}
          className={styles.img}
          onError={() => setImgError(true)}
        />
      ) : initials ? (
        <span className={styles.initials}>{initials.slice(0, 2).toUpperCase()}</span>
      ) : (
        <SilhouetteIcon />
      )}
      {status && <span className={[styles.status, styles[`status-${status}`]].join(' ')} />}
    </span>
  )
}
