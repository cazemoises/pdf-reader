/** @type {import('tailwindcss').Config} */
export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Work Sans"', "system-ui", "sans-serif"],
        serif: ['"Source Serif 4"', "Georgia", "serif"],
      },
      colors: {
        background: "var(--color-bg)",
        elevated: "var(--color-bg-elevated)",
        surface: "var(--color-surface)",
        "surface-muted": "var(--color-surface-muted)",
        border: "var(--color-border)",
        ink: "var(--color-text)",
        "ink-muted": "var(--color-text-muted)",
        "ink-faint": "var(--color-text-faint)",
        accent: {
          DEFAULT: "var(--color-accent)",
          text: "var(--color-accent-text)",
          soft: "var(--color-accent-soft)",
        },
        reading: "var(--color-reading-bg)",
        danger: {
          DEFAULT: "var(--color-error)",
          soft: "var(--color-error-soft)",
        },
        hl1: { DEFAULT: "var(--color-hl1-dot)", bg: "var(--color-hl1-bg)" },
        hl2: { DEFAULT: "var(--color-hl2-dot)", bg: "var(--color-hl2-bg)" },
        hl3: { DEFAULT: "var(--color-hl3-dot)", bg: "var(--color-hl3-bg)" },
        hl4: { DEFAULT: "var(--color-hl4-dot)", bg: "var(--color-hl4-bg)" },
      },
    },
  },
  plugins: [],
};
