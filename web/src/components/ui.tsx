import type { ComponentPropsWithRef, ReactNode } from "react";
import { cloneElement, isValidElement, useId } from "react";

type ButtonVariant = "default" | "primary" | "danger" | "quiet";

const VARIANTS: Record<ButtonVariant, string> = {
  default: "",
  primary: "btn-primary",
  danger: "btn-danger",
  quiet: "btn-quiet",
};

interface ButtonProps extends ComponentPropsWithRef<"button"> {
  variant?: ButtonVariant;
}

export function Button({
  variant = "default",
  className = "",
  type = "button",
  ...props
}: ButtonProps) {
  return (
    <button type={type} className={`btn ${VARIANTS[variant]} ${className}`.trim()} {...props} />
  );
}

type BadgeTone = "neutral" | "accent" | "success" | "warning" | "danger";

const TONES: Record<BadgeTone, string> = {
  neutral: "badge-neutral",
  accent: "badge-accent",
  success: "badge-success",
  warning: "badge-warning",
  danger: "badge-danger",
};

export function Badge({
  tone = "neutral",
  title,
  className = "",
  children,
}: {
  tone?: BadgeTone;
  title?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span className={`badge ${TONES[tone]} ${className}`.trim()} title={title}>
      {children}
    </span>
  );
}

export function SectionTitle({ children }: { children: ReactNode }) {
  return <h2 className="card-title">{children}</h2>;
}

export function EmptyState({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <p className="empty-title">{title}</p>
      {hint ? <p className="empty-hint">{hint}</p> : null}
      {children ? <div className="mt-4">{children}</div> : null}
    </div>
  );
}

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="w-full" role="status" aria-label={label}>
      <div className="loading-line" />
      <p className="feedback mt-2">{label}</p>
    </div>
  );
}

export function StatusMessage({ children }: { children: ReactNode }) {
  return (
    <p className="feedback" role="status" aria-live="polite">
      {children}
    </p>
  );
}

export function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  const id = useId();
  return (
    <div className={`grid gap-1 ${className}`}>
      <label className="label" htmlFor={id}>
        {label}
      </label>
      {isValidElement<{ id?: string }>(children) ? cloneElement(children, { id }) : children}
    </div>
  );
}

export function CheckboxField({
  label,
  checked,
  onChange,
  disabled = false,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label className="label inline-flex items-center gap-2">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      {label}
    </label>
  );
}
