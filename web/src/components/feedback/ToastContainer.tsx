import React from 'react'
import { createPortal } from 'react-dom'

type Position = 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left'

interface ToastContainerProps {
  position?: Position
  children: React.ReactNode
}

const positionStyles: Record<Position, React.CSSProperties> = {
  'top-right':    { top: 16, right: 16 },
  'top-left':     { top: 16, left: 16 },
  'bottom-right': { bottom: 16, right: 16 },
  'bottom-left':  { bottom: 16, left: 16 },
}

export function ToastContainer({ position = 'bottom-right', children }: ToastContainerProps) {
  const style: React.CSSProperties = {
    position: 'fixed',
    zIndex: 9999,
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    ...positionStyles[position],
  }

  return createPortal(
    <div style={style} aria-live="polite" aria-label="Notifications">
      {children}
    </div>,
    document.body
  )
}

export default ToastContainer
