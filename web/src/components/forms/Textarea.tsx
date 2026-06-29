import React, { useId } from 'react';
import styles from './Textarea.module.css';

type ResizeOption = 'none' | 'vertical' | 'both';

interface TextareaProps {
  label?: string;
  placeholder?: string;
  value?: string;
  defaultValue?: string;
  onChange?: (e: React.ChangeEvent<HTMLTextAreaElement>) => void;
  error?: string;
  hint?: string;
  disabled?: boolean;
  required?: boolean;
  rows?: number;
  resize?: ResizeOption;
  className?: string;
  fullWidth?: boolean;
  id?: string;
  name?: string;
}

const resizeClassMap: Record<ResizeOption, string> = {
  none: styles.resizeNone,
  vertical: styles.resizeVertical,
  both: styles.resizeBoth,
};

export const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  (
    {
      label,
      placeholder,
      value,
      defaultValue,
      onChange,
      error,
      hint,
      disabled = false,
      required = false,
      rows = 4,
      resize = 'vertical',
      className,
      fullWidth = false,
      id,
      name,
    },
    ref
  ) => {
    const generatedId = useId();
    const textareaId = id ?? generatedId;

    const wrapperClasses = [
      styles.wrapper,
      fullWidth ? styles.fullWidth : '',
      error ? styles.error : '',
      className ?? '',
    ]
      .filter(Boolean)
      .join(' ');

    const textareaClasses = [styles.textarea, resizeClassMap[resize]]
      .filter(Boolean)
      .join(' ');

    return (
      <div className={wrapperClasses}>
        {label && (
          <label htmlFor={textareaId} className={styles.label}>
            {label}
            {required && <span className={styles.required} aria-hidden="true">*</span>}
          </label>
        )}
        <textarea
          ref={ref}
          id={textareaId}
          name={name}
          value={value}
          defaultValue={defaultValue}
          onChange={onChange}
          placeholder={placeholder}
          disabled={disabled}
          required={required}
          rows={rows}
          aria-invalid={!!error}
          aria-describedby={
            error ? `${textareaId}-error` : hint ? `${textareaId}-hint` : undefined
          }
          className={textareaClasses}
        />
        {error ? (
          <span id={`${textareaId}-error`} className={styles.errorText} role="alert">
            {error}
          </span>
        ) : hint ? (
          <span id={`${textareaId}-hint`} className={styles.hintText}>
            {hint}
          </span>
        ) : null}
      </div>
    );
  }
);

Textarea.displayName = 'Textarea';
