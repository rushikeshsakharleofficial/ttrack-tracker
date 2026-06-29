import React, { useState, useId } from 'react';
import styles from './Input.module.css';

type InputSize = 'sm' | 'md' | 'lg';
type InputType = 'text' | 'password' | 'email' | 'search' | 'url' | 'number';

interface InputProps {
  label?: string;
  placeholder?: string;
  value?: string;
  defaultValue?: string;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  type?: InputType;
  error?: string;
  hint?: string;
  disabled?: boolean;
  required?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  size?: InputSize;
  id?: string;
  name?: string;
  className?: string;
  fullWidth?: boolean;
}

const SearchIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <circle cx="6.5" cy="6.5" r="4.5" stroke="currentColor" strokeWidth="1.5" />
    <path d="M10.5 10.5L14 14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

const EyeIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M1 8C1 8 3.5 3 8 3C12.5 3 15 8 15 8C15 8 12.5 13 8 13C3.5 13 1 8 1 8Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    <circle cx="8" cy="8" r="2" stroke="currentColor" strokeWidth="1.5" />
  </svg>
);

const EyeOffIcon = () => (
  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path d="M2 2L14 14M6.5 6.58A2 2 0 0 0 9.42 9.5M1 8C1 8 3.5 3 8 3C9.12 3 10.15 3.3 11.06 3.78M15 8C15 8 13.5 11.5 10.5 12.8M12.5 12.5C11.12 12.84 9.62 13 8 13C3.5 13 1 8 1 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (
    {
      label,
      placeholder,
      value,
      defaultValue,
      onChange,
      type = 'text',
      error,
      hint,
      disabled = false,
      required = false,
      leftIcon,
      rightIcon,
      size = 'md',
      id,
      name,
      className,
      fullWidth = false,
    },
    ref
  ) => {
    const [showPassword, setShowPassword] = useState(false);
    const generatedId = useId();
    const inputId = id ?? generatedId;

    const isSearch = type === 'search';
    const isPassword = type === 'password';

    const resolvedLeftIcon = isSearch ? <SearchIcon /> : leftIcon;
    const hasLeftIcon = !!resolvedLeftIcon;
    const hasRightIcon = isPassword || !!rightIcon;

    const resolvedType = isPassword ? (showPassword ? 'text' : 'password') : type;

    const sizeClass = styles[size];
    const wrapperClasses = [
      styles.wrapper,
      fullWidth ? styles.fullWidth : '',
      className ?? '',
    ]
      .filter(Boolean)
      .join(' ');

    const containerClasses = [
      styles.inputContainer,
      sizeClass,
      error ? styles.error : '',
      hasLeftIcon ? styles.hasLeftIcon : '',
      hasRightIcon ? styles.hasRightIcon : '',
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <div className={wrapperClasses}>
        {label && (
          <label htmlFor={inputId} className={styles.label}>
            {label}
            {required && <span className={styles.required} aria-hidden="true">*</span>}
          </label>
        )}
        <div className={containerClasses}>
          {resolvedLeftIcon && (
            <span className={styles.leftIcon}>{resolvedLeftIcon}</span>
          )}
          <input
            ref={ref}
            id={inputId}
            name={name}
            type={resolvedType}
            value={value}
            defaultValue={defaultValue}
            onChange={onChange}
            placeholder={placeholder}
            disabled={disabled}
            required={required}
            aria-invalid={!!error}
            aria-describedby={
              error ? `${inputId}-error` : hint ? `${inputId}-hint` : undefined
            }
            className={styles.input}
          />
          {isPassword && (
            <button
              type="button"
              className={styles.rightIconButton}
              onClick={() => setShowPassword((p) => !p)}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
              tabIndex={-1}
            >
              {showPassword ? <EyeOffIcon /> : <EyeIcon />}
            </button>
          )}
          {!isPassword && rightIcon && (
            <span className={styles.rightIcon}>{rightIcon}</span>
          )}
        </div>
        {error ? (
          <span id={`${inputId}-error`} className={styles.errorText} role="alert">
            {error}
          </span>
        ) : hint ? (
          <span id={`${inputId}-hint`} className={styles.hintText}>
            {hint}
          </span>
        ) : null}
      </div>
    );
  }
);

Input.displayName = 'Input';
