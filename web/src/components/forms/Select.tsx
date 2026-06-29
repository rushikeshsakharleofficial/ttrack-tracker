import React, { useId } from 'react';
import styles from './Select.module.css';

type SelectSize = 'sm' | 'md' | 'lg';

interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

interface SelectProps {
  label?: string;
  value?: string;
  defaultValue?: string;
  onChange?: (e: React.ChangeEvent<HTMLSelectElement>) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  error?: string;
  hint?: string;
  size?: SelectSize;
  className?: string;
  fullWidth?: boolean;
  required?: boolean;
  id?: string;
  name?: string;
}

const ChevronIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M4 6L8 10L12 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  (
    {
      label,
      value,
      defaultValue,
      onChange,
      options,
      placeholder,
      disabled = false,
      error,
      hint,
      size = 'md',
      className,
      fullWidth = false,
      required = false,
      id,
      name,
    },
    ref
  ) => {
    const generatedId = useId();
    const selectId = id ?? generatedId;

    const isPlaceholderSelected =
      value === '' || value === undefined
        ? defaultValue === '' || defaultValue === undefined
        : value === '';

    const wrapperClasses = [
      styles.wrapper,
      fullWidth ? styles.fullWidth : '',
      className ?? '',
    ]
      .filter(Boolean)
      .join(' ');

    const containerClasses = [styles.selectContainer, styles[size], error ? styles.error : '']
      .filter(Boolean)
      .join(' ');

    const selectClasses = [styles.select, isPlaceholderSelected && placeholder ? styles.placeholder : '']
      .filter(Boolean)
      .join(' ');

    return (
      <div className={wrapperClasses}>
        {label && (
          <label htmlFor={selectId} className={styles.label}>
            {label}
            {required && <span className={styles.required} aria-hidden="true">*</span>}
          </label>
        )}
        <div className={containerClasses}>
          <select
            ref={ref}
            id={selectId}
            name={name}
            value={value}
            defaultValue={defaultValue ?? (placeholder ? '' : undefined)}
            onChange={onChange}
            disabled={disabled}
            required={required}
            aria-invalid={!!error}
            aria-describedby={
              error ? `${selectId}-error` : hint ? `${selectId}-hint` : undefined
            }
            className={selectClasses}
          >
            {placeholder && (
              <option value="" disabled>
                {placeholder}
              </option>
            )}
            {options.map((opt) => (
              <option key={opt.value} value={opt.value} disabled={opt.disabled}>
                {opt.label}
              </option>
            ))}
          </select>
          <span className={styles.chevron}>
            <ChevronIcon />
          </span>
        </div>
        {error ? (
          <span id={`${selectId}-error`} className={styles.errorText} role="alert">
            {error}
          </span>
        ) : hint ? (
          <span id={`${selectId}-hint`} className={styles.hintText}>
            {hint}
          </span>
        ) : null}
      </div>
    );
  }
);

Select.displayName = 'Select';
