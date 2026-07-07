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
        <nav className="flex items-center gap-1 overflow-x-auto border-b pb-2 mb-6">
            {routes.map(route => {
                const Icon = route.icon;
                return (
                    <Link
                        key={route.href}
                        href={route.href}
                        className={cn(
                            "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-muted-foreground transition-all hover:text-primary whitespace-nowrap",
                            pathname.startsWith(route.href) && "bg-muted text-primary"
                        )}
                    >
                        <Icon className="h-4 w-4" />
                        {route.label}
                    </Link>
                )
            })}
        </nav>
    );
}
