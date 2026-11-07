import {
  Button,
  Description,
  FieldError,
  InputGroup,
  Label,
  TextField,
} from "@heroui/react";
import { Eye, EyeOff, LockKeyhole } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";
import { useState } from "react";

type AuthFieldProps = {
  name: string;
  label: string;
  placeholder?: string;
  icon?: ReactNode;
  description?: string;
  autoComplete?: string;
  type?: "text" | "url" | "password";
  defaultValue?: string;
  isDisabled?: boolean;
  validate?: ComponentProps<typeof TextField>["validate"];
};

export function AuthField({
  name,
  label,
  placeholder,
  icon,
  description,
  autoComplete,
  type = "text",
  defaultValue,
  isDisabled,
  validate,
}: AuthFieldProps) {
  const [visible, setVisible] = useState(false);
  const password = type === "password";
  return (
    <TextField
      className="auth-field"
      name={name}
      isRequired
      isDisabled={isDisabled ?? false}
      defaultValue={defaultValue ?? ""}
      {...(autoComplete ? { autoComplete } : {})}
      {...(validate ? { validate } : {})}
    >
      <Label>{label}</Label>
      <InputGroup className="auth-input">
        <InputGroup.Prefix>
          {password ? <LockKeyhole aria-hidden="true" /> : icon}
        </InputGroup.Prefix>
        <InputGroup.Input
          type={password ? (visible ? "text" : "password") : type}
          placeholder={placeholder ?? ""}
        />
        {password && (
          <InputGroup.Suffix>
            <Button
              type="button"
              className="password-toggle"
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={`${visible ? "隐藏" : "显示"}${label}`}
              aria-pressed={visible}
              onPress={() => setVisible(!visible)}
            >
              {visible ? (
                <Eye aria-hidden="true" />
              ) : (
                <EyeOff aria-hidden="true" />
              )}
            </Button>
          </InputGroup.Suffix>
        )}
      </InputGroup>
      {description && <Description>{description}</Description>}
      <FieldError />
    </TextField>
  );
}
