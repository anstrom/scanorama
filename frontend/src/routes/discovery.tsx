import { RootLayout } from "../components/layout/root-layout";

export function DiscoveryPage() {
  return (
    <RootLayout title="Discovery">
      <div className="bg-surface rounded-lg border border-border p-6 text-center">
        <p className="text-text-secondary text-sm">Discovery jobs coming soon.</p>
      </div>
    </RootLayout>
  );
}
