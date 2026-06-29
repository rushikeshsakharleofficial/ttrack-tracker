import React, { useId } from 'react';
import styles from './Radio.module.css';

interface RadioProps {
  label?: string;
  value: string;
  checked?: boolean;
  defaultChecked?: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  disabled?: boolean;
  name?: string;
  id?: string;
  className?: string;
}

export const Radio: React.FC<RadioProps> = ({
  label,
  value,
  checked,
  defaultChecked,
  onChange,
  disabled = false,
  name,
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
        disabled ? styles.disabled : '',
        className ?? '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <input
        type="radio"
        id={inputId}
        name={name}
        value={value}
        checked={checked}
        defaultChecked={defaultChecked}
        onChange={onChange}
        disabled={disabled}
        className={styles.nativeInput}
      />
      <span className={styles.circle} aria-hidden="true" />
      {label && <span className={styles.label}>{label}</span>}
    </label>
  );
};
