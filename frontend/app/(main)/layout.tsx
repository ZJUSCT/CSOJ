"use client";
import withAuth from "@/components/layout/with-auth";
import { MainNav } from "@/components/layout/main-nav";
import { UserNav } from "@/components/layout/user-nav";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import {useTranslations} from 'next-intl';
import { LanguageToggle } from "@/components/layout/lang-toggle";

function MainLayout({ children }: { children: React.ReactNode }) {
  const t = useTranslations('home');

  return (
    <div className="flex min-h-screen w-full flex-col">
      <header className="sticky top-0 flex h-16 items-center gap-4 border-b bg-background px-12 md:px-24 z-50">
        <MainNav />
        <div className="flex w-full items-center gap-4 md:ml-auto md:gap-2 lg:gap-4">
          <div className="ml-auto flex-1 sm:flex-initial">
          </div>
          <LanguageToggle />
          <ThemeToggle />
          <UserNav />
        </div>
      </header>
      <main className="flex flex-1 flex-col gap-4 p-8 px-12 md:gap-8 md:p-8 md:px-24">
        {children}
      </main>
      <footer className="mt-auto border-t py-4">
        <div className="container mx-auto text-center text-sm text-muted-foreground">
          {t.rich('power_by', {
            github1: (chunks) => (
              <a
                href="https://github.com/ZJUSCT/CSOJ"
                target="_blank"
                rel="noopener noreferrer"
                className="font-medium text-primary underline-offset-4 hover:underline"
              >
                {chunks}
              </a>
            ),
            github2: (chunks) => (
              <a
                href="https://github.com/ZJUSCT/CSOJ-WebUI"
                target="_blank"
                rel="noopener noreferrer"
                className="font-medium text-primary underline-offset-4 hover:underline"
              >
                {chunks}
              </a>
            ),
          })}
        </div>
      </footer>
    </div>
  );
}

export default withAuth(MainLayout);