interface PlaceholderPageProps {
  message: string;
}

export function PlaceholderPage({ message }: PlaceholderPageProps) {
  return (
    <div className="bg-surface rounded-lg border border-border p-6 text-center">
      <p className="text-text-secondary text-sm">{message}</p>
    </div>
  );
}
