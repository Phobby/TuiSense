// Small inline icons and the logo (no icon font, nothing loaded remotely).

export function Logo() {
  return (
    <svg className="logo" viewBox="0 0 64 64" aria-hidden="true">
      <defs>
        <linearGradient id="logo-g" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#5b8cff" />
          <stop offset="1" stopColor="#9b7bff" />
        </linearGradient>
      </defs>
      <path
        d="M20 22h24c6 0 9 5 10 11l2 9c1 5-4 8-8 4l-5-5H21l-5 5c-4 4-9 1-8-4l2-9c1-6 4-11 10-11z"
        fill="none"
        stroke="url(#logo-g)"
        strokeWidth="3.5"
        strokeLinejoin="round"
      />
      <circle cx="24" cy="31" r="3" fill="#5b8cff" />
      <circle cx="40" cy="31" r="3" fill="#9b7bff" />
    </svg>
  );
}

const icon = { width: 16, height: 16, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true } as const;

export const IconArrow = () => (
  <svg {...icon}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </svg>
);
export const IconBack = () => (
  <svg {...icon}>
    <path d="M19 12H5M11 6l-6 6 6 6" />
  </svg>
);
export const IconGitHub = () => (
  <svg {...icon}>
    <path d="M9 19c-4.3 1.4-4.3-2.5-6-3m12 5v-3.5c0-1 .1-1.4-.5-2 2.8-.3 5.5-1.4 5.5-6a4.6 4.6 0 0 0-1.3-3.2 4.2 4.2 0 0 0-.1-3.2s-1.1-.3-3.5 1.3a12.3 12.3 0 0 0-6.2 0C6.5 2.8 5.4 3.1 5.4 3.1a4.2 4.2 0 0 0-.1 3.2A4.6 4.6 0 0 0 4 9.5c0 4.6 2.7 5.7 5.5 6-.6.6-.6 1.2-.5 2V21" />
  </svg>
);
export const IconDownload = () => (
  <svg {...icon}>
    <path d="M12 3v12M7 10l5 5 5-5M5 21h14" />
  </svg>
);
export const IconRestart = () => (
  <svg {...icon}>
    <path d="M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5" />
  </svg>
);
export const IconReset = () => (
  <svg {...icon}>
    <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14M10 11v6M14 11v6" />
  </svg>
);
export const IconFullscreen = () => (
  <svg {...icon}>
    <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
  </svg>
);
export const IconExitFullscreen = () => (
  <svg {...icon}>
    <path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5" />
  </svg>
);
