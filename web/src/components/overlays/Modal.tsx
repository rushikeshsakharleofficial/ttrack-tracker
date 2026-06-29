import React, { useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import styles from './Modal.module.css'

type ModalSize = 'sm' | 'md' | 'lg' | 'xl' | 'full'

interface ModalProps {
  open: boolean
  onClose: () => void
  title?: string
  children: React.ReactNode
  footer?: React.ReactNode
  size?: ModalSize
  closeOnBackdrop?: boolean
  closeOnEsc?: boolean
  className?: string
}

const FOCUSABLE = 'a[href],button:not([disabled]),textarea,input,select,[tabindex]:not([tabindex="-1"])'

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  size = 'md',
  closeOnBackdrop = true,
  closeOnEsc = true,
  className,
}: ModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const prevFocusRef = useRef<Element | null>(null)

  // Trap focus
  const trapFocus = useCallback((e: KeyboardEvent) => {
    if (e.key !== 'Tab' || !dialogRef.current) return
    const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE))
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (e.shiftKey) {
      if (document.activeElement === first) { e.preventDefault(); last.focus() }
    } else {
      if (document.activeElement === last) { e.preventDefault(); first.focus() }
    }
  }, [])

  const handleKey = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape' && closeOnEsc) onClose()
    trapFocus(e)
  }, [closeOnEsc, onClose, trapFocus])

  useEffect(() => {
    if (open) {
      prevFocusRef.current = document.activeElement
      document.addEventListener('keydown', handleKey)
      document.body.style.overflow = 'hidden'
      // focus first focusable inside
      requestAnimationFrame(() => {
        const first = dialogRef.current?.querySelector<HTMLElement>(FOCUSABLE)
        first?.focus()
      })
    } else {
      document.removeEventListener('keydown', handleKey)
      document.body.style.overflow = ''
      ;(prevFocusRef.current as HTMLElement | null)?.focus()
    }
    return () => {
      document.removeEventListener('keydown', handleKey)
      document.body.style.overflow = ''
    }
  }, [open, handleKey])

  if (!open) return null

  return createPortal(
    <div
      className={styles.backdrop}
      onClick={closeOnBackdrop ? onClose : undefined}
      aria-modal="true"
      role="presentation"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? 'modal-title' : undefined}
        className={[styles.modal, styles[`size-${size}`], className].filter(Boolean).join(' ')}
        onClick={(e) => e.stopPropagation()}
      >
        {(title != null) && (
          <>
            <div className={styles.header}>
              <h2 id="modal-title" className={styles.title}>{title}</h2>
              <button
                type="button"
                className={styles.close}
                onClick={onClose}
                aria-label="Close"
              >
                <svg width="20" height="20" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                  <path d="M15 5L5 15M5 5l10 10" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
                </svg>
              </button>
            </div>
            <div className={styles.divider} />
          </>
        )}
        <div className={styles.body}>{children}</div>
        {footer && (
          <>
            <div className={styles.divider} />
            <div className={styles.footer}>{footer}</div>
          </>
        )}
      </div>
    </div>,
    document.body,
  )
}
