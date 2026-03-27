export function MareWordmark({ className }: { className?: string }) {
  return (
    <span className={`mare-wordmark${className ? ` ${className}` : ""}`} aria-label="MARE">
      <span>M</span>
      <span>A</span>
      <span>R</span>
      <span>E</span>
    </span>
  );
}
