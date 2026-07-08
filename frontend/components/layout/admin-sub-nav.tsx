"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
    Users,
    Server,
    FileCode,
    Trophy,
    BookCopy,
    Package,
    Settings,
    ClipboardCheck,
} from "lucide-react"
import { cn } from "@/lib/utils";

const routes = [
    { href: "/admin/cluster", label: "Cluster", icon: Server },
    { href: "/admin/users", label: "Users", icon: Users },
    { href: "/admin/submissions", label: "Submissions", icon: FileCode },
    { href: "/admin/containers", label: "Containers", icon: Package },
    { href: "/admin/contests", label: "Contests", icon: Trophy },
    { href: "/admin/registrations", label: "Registrations", icon: ClipboardCheck },
    { href: "/admin/problems", label: "Problems", icon: BookCopy },
    { href: "/admin/settings", label: "Settings", icon: Settings },
];

export function AdminSubNav() {
    const pathname = usePathname();

    return (
        <aside className="hidden md:flex md:w-[220px] md:flex-col md:gap-1 border-r md:min-h-[calc(100vh-3.5rem)] md:sticky md:top-14">
            <div className="flex h-14 items-center border-b px-4">
                <span className="font-semibold text-sm">Admin Panel</span>
            </div>
            <nav className="flex flex-col gap-0.5 p-2">
                {routes.map(route => {
                    const Icon = route.icon;
                    const active = pathname.startsWith(route.href);
                    return (
                        <Link
                            key={route.href}
                            href={route.href}
                            className={cn(
                                "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all",
                                active
                                    ? "bg-primary/10 text-primary"
                                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                            )}
                        >
                            <Icon className="h-4 w-4 shrink-0" />
                            {route.label}
                        </Link>
                    )
                })}
            </nav>
        </aside>
    );
}
