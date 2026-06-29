import React, { useId } from 'react';
import styles from './Toggle.module.css';

type ToggleSize = 'sm' | 'md';

interface ToggleProps {
  checked?: boolean;
  defaultChecked?: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  disabled?: boolean;
  label?: string;
  size?: ToggleSize;
  id?: string;
  className?: string;
}

export const Toggle: React.FC<ToggleProps> = ({
  checked,
  defaultChecked,
  onChange,
  disabled = false,
  label,
  size = 'md',
  id,
  className,
}) => {
  const generatedId = useId();
  const inputId = id ?? generatedId;

  return (
    <label
      htmlFor={inputId}
      className={[
        styles.wrapper,
        styles[size],
        disabled ? styles.disabled : '',
        className ?? '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <input
        type="checkbox"
        role="switch"
        id={inputId}
        checked={checked}
        defaultChecked={defaultChecked}
        onChange={onChange}
        disabled={disabled}
        aria-checked={checked}
        className={styles.nativeInput}
      />
      <span className={styles.track} aria-hidden="true" />
      {label && <span className={styles.label}>{label}</span>}
    </label>
  );
};
