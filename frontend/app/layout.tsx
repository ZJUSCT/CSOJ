import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { cn } from "@/lib/utils";
import { SWRProvider } from "@/providers/swr-provider";
import { AuthProvider } from "@/providers/auth-provider";
import { Toaster } from "@/components/ui/toaster";
import { ThemeProvider } from "@/providers/theme-provider";
// import {NextIntlClientProvider} from 'next-intl';
import { ClientIntlProvider } from "@/providers/i18n-provider";

const inter = Inter({ subsets: ["latin"], variable: "--font-sans" });

export const metadata: Metadata = {
  title: "CSOJ - Online Judge",
  description: "A Fully Containerized Secure Online Judgement System",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script
          id="mathjax-script"
          async
          src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"
        ></script>
      </head>
      <body
        className={cn(
          "min-h-screen bg-background font-sans antialiased",
          inter.variable
        )}
      >
        <ClientIntlProvider>
          <ThemeProvider
            attribute="class"
            defaultTheme="system"
            enableSystem
          >
            <AuthProvider>
              <SWRProvider>{children}</SWRProvider>
            </AuthProvider>
            <Toaster />
          </ThemeProvider>
        </ClientIntlProvider>
      </body>
    </html>
  );
}