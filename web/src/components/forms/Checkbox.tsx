import React, { useId, useRef, useEffect } from 'react';
import styles from './Checkbox.module.css';

interface CheckboxProps {
  label?: string;
  checked?: boolean;
  defaultChecked?: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  disabled?: boolean;
  indeterminate?: boolean;
  error?: string;
  id?: string;
  name?: string;
  className?: string;
}

export const Checkbox: React.FC<CheckboxProps> = ({
  label,
  checked,
  defaultChecked,
  onChange,
  disabled = false,
  indeterminate = false,
  error,
  id,
  name,
  className,
}) => {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (ref.current) {
      ref.current.indeterminate = indeterminate;
    }
  }, [indeterminate]);

  return (
    <div className={className}>
      <label
        htmlFor={inputId}
        className={[styles.wrapper, disabled ? styles.disabled : ''].filter(Boolean).join(' ')}
      >
        <input
          ref={ref}
          type="checkbox"
          id={inputId}
          name={name}
          checked={checked}
          defaultChecked={defaultChecked}
          onChange={onChange}
          disabled={disabled}
          aria-invalid={!!error}
          aria-describedby={error ? `${inputId}-error` : undefined}
          className={styles.nativeInput}
        />
        <span className={styles.box} aria-hidden="true" />
        {label && <span className={styles.label}>{label}</span>}
      </label>
      {error && (
        <span id={`${inputId}-error`} className={styles.errorText} role="alert">
          {error}
        </span>
      )}
    </div>
  );
};
