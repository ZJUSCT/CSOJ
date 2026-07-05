"use client";

import * as React from "react";
import { Moon, Sun, Laptop } from "lucide-react";
import { useTheme } from "next-themes";
import { useTranslations } from "next-intl"; // Import useTranslations

import { Button } from "@/components/ui/button";

export function ThemeToggle() {
  const t = useTranslations('home.theme'); // Initialize translations
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = React.useState(false);

  // useEffect only runs on the client, so now we can safely show the UI
  React.useEffect(() => {
    setMounted(true);
  }, []);

  const cycleTheme = () => {
    if (theme === "light") {
      setTheme("dark");
    } else if (theme === "dark") {
      setTheme("system");
    } else {
      setTheme("light");
    }
  };

  // Avoid rendering the button on the server to prevent hydration mismatch
  if (!mounted) {
    // Keep the disabled button for layout stability during hydration
    return <Button variant="outline" size="icon" disabled={true} />; 
  }

  // Determine the label based on the current theme for accessibility/tooltip
  let label = "";
  if (theme === "light") {
    label = t('toggleToDark');
  } else if (theme === "dark") {
    label = t('toggleToSystem');
  } else {
    label = t('toggleToLight');
  }

  return (
    <Button 
      variant="outline" 
      size="icon" 
      onClick={cycleTheme}
      title={label} // Optional: add title for better UX
    >
      {theme === "light" && <Sun className="h-[1.2rem] w-[1.2rem]" />}
      {theme === "dark" && <Moon className="h-[1.2rem] w-[1.2rem]" />}
      {theme === "system" && <Laptop className="h-[1.2rem] w-[1.2rem]" />}
      <span className="sr-only">{t('toggleTheme')}</span>
    </Button>
  );
}