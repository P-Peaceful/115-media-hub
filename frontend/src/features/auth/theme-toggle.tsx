import { Button, useTheme } from "@heroui/react";
import { Moon, Sun } from "lucide-react";

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme("system");
  const dark = resolvedTheme === "dark";

  return (
    <Button
      className="theme-toggle"
      variant="secondary"
      isIconOnly
      aria-label={`切换到${dark ? "亮色" : "暗色"}主题`}
      onPress={() => setTheme(dark ? "light" : "dark")}
    >
      {dark ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
    </Button>
  );
}
