"use client";
import { useAuth } from "@/hooks/use-auth";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { Loader2 } from "lucide-react";

const withAdmin = <P extends object>(Component: React.ComponentType<P>) => {
  const AdminComponent = (props: P) => {
    const { user, isAuthenticated, isLoading } = useAuth();
    const router = useRouter();
    const isAdmin = user?.role === "admin" || user?.role === "superadmin";

    useEffect(() => {
      if (!isLoading && (!isAuthenticated || !isAdmin)) {
        router.push("/contests");
      }
    }, [isAuthenticated, isLoading, isAdmin, router]);

    if (isLoading) {
      return (
        <div className="flex h-screen items-center justify-center">
          <Loader2 className="h-12 w-12 animate-spin" />
        </div>
      );
    }

    if (!isAuthenticated || !isAdmin) {
      return null;
    }

    return <Component {...props} />;
  };

  AdminComponent.displayName = `withAdmin(${
    Component.displayName || Component.name || "Component"
  })`;

  return AdminComponent;
};

export default withAdmin;
