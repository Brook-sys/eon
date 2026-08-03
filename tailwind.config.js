/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./internal/dashboard/views/**/*.templ",
    "./internal/dashboard/views/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        bg: 'var(--bg)',
        panel: 'var(--panel)',
        'panel-2': 'var(--panel-2)',
        border: 'var(--border)',
        text: 'var(--text)',
        muted: 'var(--muted)',
        accent: 'var(--accent)',
        ok: 'var(--ok)',
        warn: 'var(--warn)',
        err: 'var(--err)',
      },
      fontFamily: {
        sans: 'var(--sans)',
        mono: 'var(--mono)',
      },
    },
  },
  plugins: [],
}
