import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

interface RootLayoutProps {
  title?: string;
  children: React.ReactNode;
}

export function RootLayout({ title = "Dashboard", children }: RootLayoutProps) {
  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar />
      <div className="flex flex-col flex-1 min-w-0">
        <Topbar title={title} />
        <main className="flex-1 overflow-y-auto p-4">
          {children}
        </main>
      </div>
    </div>
  );
}
