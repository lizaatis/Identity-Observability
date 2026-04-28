/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        risk: {
          critical: '#dc2626',
          high: '#ea580c',
          medium: '#f59e0b',
          low: '#10b981',
        },
        brand: {
          ink: '#020617',       // deep slate / backdrop
          surface: '#020617',   // base background
          panel: '#020617',     // cards use own bg
          accent: '#22d3ee',    // cyan
          accentSoft: '#0f172a',
          accentWarm: '#f97316',
        },
      },
    },
  },
  plugins: [],
}
