import clsx from "clsx";

export function MareLogo({ className }: { className?: string }) {
  return (
    <div className={clsx("mare-logo", className)} aria-hidden="true">
      <svg viewBox="0 0 72 72" role="img">
        <title>Mare logo</title>
        <rect x="3" y="3" width="66" height="66" rx="20" className="mare-logo-surface" />
        <circle cx="53" cy="18" r="7" className="mare-logo-sun" />
        <path
          className="mare-logo-wave-back"
          d="M7 49c7-7 13-10 18-10 7 0 10 5 15 9 4 4 9 6 15 6 5 0 10-2 17-6v17H7V49Z"
        />
        <path
          className="mare-logo-wave-main"
          d="M4 43c5-7 11-11 18-11 6 0 10 3 14 8 3 4 6 6 10 6 5 0 8-3 12-7 4-5 8-8 14-8v12c-3-2-6-3-10-3-4 0-7 1-11 4-5 4-10 6-17 6-8 0-13-3-17-7-4-4-7-6-11-6-4 0-8 2-12 6v0Z"
        />
        <path
          className="mare-logo-foam"
          d="M8 34c6-4 10-6 15-6 5 0 8 2 11 6 2 3 4 4 7 4 4 0 6-2 9-5 3-3 7-5 12-5 5 0 8 2 10 5-4-1-7 0-10 3-4 4-7 8-12 10-4 2-8 2-12 1-4-1-6-3-9-6-3-4-6-6-10-6-4 0-8 1-11 5v-6Z"
        />
        <path
          className="mare-logo-line"
          d="M12 22c4-3 8-4 12-4 5 0 8 2 12 5"
        />
      </svg>
    </div>
  );
}
