"use client";
import withAuth from "@/components/layout/with-auth";
import { MainNav } from "@/components/layout/main-nav";
import { UserNav } from "@/components/layout/user-nav";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/lang-toggle";
import { AdminSubNav } from "@/components/layout/admin-sub-nav";

function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen w-full flex-col">
      <header className="sticky top-0 flex h-16 items-center gap-4 border-b bg-background px-6 z-50">
        <MainNav />
        <div className="flex w-full items-center gap-4 md:ml-auto md:gap-2 lg:gap-4">
          <div className="ml-auto flex-1 sm:flex-initial"></div>
          <LanguageToggle />
          <ThemeToggle />
          <UserNav />
        </div>
      </header>
      <div className="flex flex-1 overflow-hidden">
        <AdminSubNav />
        <main className="flex-1 overflow-auto p-6">
          {children}
        </main>
      </div>
    </div>
  );
}

export default withAuth(AdminLayout);
