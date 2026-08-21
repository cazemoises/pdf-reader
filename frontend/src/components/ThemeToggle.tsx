import { useTheme } from "../hooks/useTheme";

function ThemeToggle() {
  const [theme, toggleTheme] = useTheme();
  const isDark = theme === "dark";

  return (
    <button
      type="button"
      onClick={toggleTheme}
      aria-label={isDark ? "Mudar para tema claro" : "Mudar para tema escuro"}
      aria-pressed={isDark}
      className="relative flex h-6 w-11 shrink-0 items-center rounded-full border border-border bg-surface-muted"
    >
      <span
        className="absolute top-px h-5 w-5 rounded-full bg-accent transition-[left] duration-150"
        style={{ left: isDark ? "22px" : "2px" }}
      />
    </button>
  );
}

export default ThemeToggle;
