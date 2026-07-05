"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import useSWR from 'swr';
import { cn } from "@/lib/utils";
import { CodeXml } from "lucide-react";
import api from '@/lib/api';
import { LinkItem } from '@/lib/types';
import { useTranslations } from "next-intl";

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

export function MainNav({ className, ...props }: React.HTMLAttributes<HTMLElement>) {
  const pathname = usePathname();
  const t = useTranslations('home');
  const { data: dynamicLinks } = useSWR<LinkItem[]>('/links', fetcher, {
    revalidateOnFocus: false,
  });

  const allRoutes = [
    { href: "/contests", label: t("contests") },
    { href: "/submissions", label: t("submissions") },
    { href: "/profile", label: t("profile") },
    ...(dynamicLinks?.map(link => ({ href: link.url, label: link.name })) || []),
  ];

  return (
    <nav className={cn("flex items-center space-x-4 lg:space-x-6", className)} {...props}>
      <Link href="/contests" className="flex items-center gap-2 font-semibold">
        <CodeXml className="h-6 w-6" />
        <span className="">CSOJ</span>
      </Link>
      {allRoutes.map((route) => {
        const isExternal = route.href.startsWith("http");
        const isActive = !isExternal && pathname.startsWith(route.href);

        if (isExternal) {
          return (
            <a
              key={route.href}
              href={route.href}
              target="_blank"
              rel="noopener noreferrer"
              className={cn(
                "text-sm font-medium transition-colors hover:text-primary",
                "text-muted-foreground",
                "whitespace-nowrap"
              )}
            >
              {route.label}
            </a>
          );
        }

        return (
          <Link
            key={route.href}
            href={route.href}
            className={cn(
              "text-sm font-medium transition-colors hover:text-primary",
              isActive ? "text-primary" : "text-muted-foreground",
              "whitespace-nowrap"
            )}
          >
            {route.label}
          </Link>
        );
      })}
    </nav>
  );
}