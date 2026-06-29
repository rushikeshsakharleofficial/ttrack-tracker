import React, { useMemo } from 'react'
import styles from './Pagination.module.css'

interface PaginationProps {
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
  siblingCount?: number
  showFirstLast?: boolean
  className?: string
}

function range(start: number, end: number): number[] {
  return Array.from({ length: end - start + 1 }, (_, i) => start + i)
}

export function Pagination({
  currentPage,
  totalPages,
  onPageChange,
  siblingCount = 1,
  showFirstLast = true,
  className,
}: PaginationProps) {
  const pages = useMemo(() => {
    const total = 2 * siblingCount + 5 // first + last + siblings*2 + current + 2 ellipsis
    if (totalPages <= total) return range(1, totalPages)

    const leftSibling = Math.max(currentPage - siblingCount, 1)
    const rightSibling = Math.min(currentPage + siblingCount, totalPages)
    const showLeftDots = leftSibling > 2
    const showRightDots = rightSibling < totalPages - 1

    if (!showLeftDots && showRightDots) {
      const leftRange = range(1, 3 + 2 * siblingCount)
      return [...leftRange, '...', totalPages]
    }
    if (showLeftDots && !showRightDots) {
      const rightRange = range(totalPages - (3 + 2 * siblingCount) + 1, totalPages)
      return [1, '...', ...rightRange]
    }
    return [1, '...', ...range(leftSibling, rightSibling), '...', totalPages]
  }, [currentPage, totalPages, siblingCount])

  if (totalPages <= 1) return null

  const go = (p: number) => {
    if (p >= 1 && p <= totalPages && p !== currentPage) onPageChange(p)
  }

  return (
    <nav aria-label="Pagination" className={[styles.pagination, className].filter(Boolean).join(' ')}>
      {showFirstLast && (
        <button
          className={styles.btn}
          onClick={() => go(1)}
          disabled={currentPage === 1}
          aria-label="First page"
        >
          «
        </button>
      )}
      <button
        className={styles.btn}
        onClick={() => go(currentPage - 1)}
        disabled={currentPage === 1}
        aria-label="Previous page"
      >
        ‹
      </button>

      {pages.map((p, i) =>
        p === '...' ? (
          <span key={`ellipsis-${i}`} className={styles.ellipsis}>
            …
          </span>
        ) : (
          <button
            key={p}
            className={[styles.btn, p === currentPage ? styles.active : ''].filter(Boolean).join(' ')}
            onClick={() => go(p as number)}
            aria-label={`Page ${p}`}
            aria-current={p === currentPage ? 'page' : undefined}
          >
            {p}
          </button>
        )
      )}

      <button
        className={styles.btn}
        onClick={() => go(currentPage + 1)}
        disabled={currentPage === totalPages}
        aria-label="Next page"
      >
        ›
      </button>
      {showFirstLast && (
        <button
          className={styles.btn}
          onClick={() => go(totalPages)}
          disabled={currentPage === totalPages}
          aria-label="Last page"
        >
          »
        </button>
      )}
    </nav>
  )
}
