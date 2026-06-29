import React, { useState } from 'react'
import { Button } from './components/buttons/Button'

const variants = ['primary', 'secondary', 'outline', 'ghost', 'danger'] as const
const sizes = ['sm', 'md', 'lg'] as const

const sectionStyle: React.CSSProperties = {
  marginBottom: 40,
}

const headingStyle: React.CSSProperties = {
  fontFamily: 'system-ui, sans-serif',
  fontSize: 13,
  fontWeight: 600,
  color: '#6B7280',
  textTransform: 'uppercase',
  letterSpacing: '0.05em',
  marginBottom: 16,
}

const rowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 12,
  flexWrap: 'wrap',
  marginBottom: 12,
}

const labelStyle: React.CSSProperties = {
  fontFamily: 'system-ui, sans-serif',
  fontSize: 12,
  color: '#9CA3AF',
  width: 80,
  flexShrink: 0,
}

export default function App() {
  const [loading, setLoading] = useState(false)

  function handleLoadingDemo() {
    setLoading(true)
    setTimeout(() => setLoading(false), 2000)
  }

  return (
    <div style={{ padding: 40, maxWidth: 900, margin: '0 auto' }}>
      <h1
        style={{
          fontFamily: 'system-ui, sans-serif',
          fontSize: 24,
          fontWeight: 700,
          color: '#111827',
          marginBottom: 8,
        }}
      >
        UI Component Library
      </h1>
      <p
        style={{
          fontFamily: 'system-ui, sans-serif',
          fontSize: 14,
          color: '#6B7280',
          marginBottom: 40,
        }}
      >
        Design token showcase — ttrack-tracker
      </p>

      {/* Button variants × sizes */}
      <section style={sectionStyle}>
        <p style={headingStyle}>Button — variants × sizes</p>
        {sizes.map((size) => (
          <div key={size} style={rowStyle}>
            <span style={labelStyle}>{size}</span>
            {variants.map((variant) => (
              <Button key={variant} variant={variant} size={size}>
                {variant}
              </Button>
            ))}
          </div>
        ))}
      </section>

      {/* Loading state */}
      <section style={sectionStyle}>
        <p style={headingStyle}>Button — loading</p>
        <div style={rowStyle}>
          <Button loading>Saving…</Button>
          <Button variant="outline" loading>Uploading…</Button>
          <Button variant="danger" loading size="sm">Deleting…</Button>
          <Button onClick={handleLoadingDemo} loading={loading}>
            {loading ? 'Working…' : 'Click to demo'}
          </Button>
        </div>
      </section>

      {/* Disabled state */}
      <section style={sectionStyle}>
        <p style={headingStyle}>Button — disabled</p>
        <div style={rowStyle}>
          {variants.map((variant) => (
            <Button key={variant} variant={variant} disabled>
              {variant}
            </Button>
          ))}
        </div>
      </section>

      {/* Icons */}
      <section style={sectionStyle}>
        <p style={headingStyle}>Button — with icons</p>
        <div style={rowStyle}>
          <Button leftIcon={<span>+</span>}>New item</Button>
          <Button variant="outline" rightIcon={<span>→</span>}>Continue</Button>
          <Button variant="ghost" leftIcon={<span>↓</span>} rightIcon={<span>↑</span>}>Sort</Button>
          <Button variant="danger" leftIcon={<span>✕</span>}>Remove</Button>
        </div>
      </section>

      {/* Full width */}
      <section style={sectionStyle}>
        <p style={headingStyle}>Button — fullWidth</p>
        <div style={{ maxWidth: 320 }}>
          <Button fullWidth>Full width primary</Button>
          <div style={{ marginTop: 8 }}>
            <Button variant="outline" fullWidth>Full width outline</Button>
          </div>
        </div>
      </section>
    </div>
  )
}
