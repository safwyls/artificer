import { useEffect, useRef, useState } from "react";
import { Input } from "./input";

interface NumberInputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type"> {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  /** Value committed when the field is left blank (defaults to min, else 0). */
  emptyValue?: number;
}

/**
 * A controlled number input that lets the field go empty while you type.
 *
 * A plain `<input type="number">` bound to a number snaps to 0 the instant you
 * clear it, which makes replacing a value a fight. This keeps the raw text
 * locally so the box can be blank, only emits a number for valid input, and
 * normalizes (clamp + fill blanks) on blur.
 */
export function NumberField({ value, onChange, min, max, emptyValue, onFocus, onBlur, ...rest }: NumberInputProps) {
  const [text, setText] = useState(() => String(value));
  const editing = useRef(false);

  // Reflect external changes (e.g. a save prefill) unless mid-edit.
  useEffect(() => {
    if (!editing.current) setText(String(value));
  }, [value]);

  const clamp = (n: number) => {
    if (min !== undefined) n = Math.max(min, n);
    if (max !== undefined) n = Math.min(max, n);
    return n;
  };

  return (
    <Input
      {...rest}
      type="number"
      inputMode="numeric"
      min={min}
      max={max}
      value={text}
      onFocus={(e) => {
        editing.current = true;
        onFocus?.(e);
      }}
      onChange={(e) => {
        const raw = e.target.value;
        setText(raw);
        if (raw === "") return; // stay blank while typing; don't push 0
        const n = Number(raw);
        if (!Number.isNaN(n)) onChange(n);
      }}
      onBlur={(e) => {
        editing.current = false;
        const committed = text === "" ? (emptyValue ?? min ?? 0) : clamp(Number(text) || 0);
        setText(String(committed));
        onChange(committed);
        onBlur?.(e);
      }}
    />
  );
}
