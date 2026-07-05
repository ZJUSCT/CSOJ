"use client";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import Link from "next/link";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import api from "@/lib/api";
import { useRouter } from "next/navigation";
import { useToast } from "@/hooks/use-toast";
import useSWR from "swr";
import { AuthStatus } from "@/lib/types";
import { Skeleton } from "../ui/skeleton";
import { useTranslations } from "next-intl";

const fetcher = (url: string) => api.get(url).then(res => res.data.data);


export function RegisterForm() {
  const t = useTranslations('auth.register');
  const router = useRouter();
  const { toast } = useToast();
  
  // Define schema using t() for localized error messages
  const formSchema = z.object({
    username: z.string().min(3, t('form.usernameMinLength')),
    nickname: z.string().min(1, t('form.nicknameRequired')),
    password: z.string().min(6, t('form.passwordMinLength')),
  });

  const { data: authStatus, isLoading } = useSWR<AuthStatus>('/auth/status', fetcher);

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      username: "",
      nickname: "",
      password: "",
    },
  });

  const onSubmit = async (values: z.infer<typeof formSchema>) => {
    try {
      const response = await api.post("/auth/local/register", values);
      if (response.data.code === 0) {
        toast({
          title: t('toast.successTitle'),
          description: t('toast.successDescription'),
        });
        router.push("/login");
      } else {
        throw new Error(response.data.message || t('toast.failDefault'));
      }
    } catch (error: any) {
      toast({
        variant: "destructive",
        title: t('toast.failTitle'),
        description: error.response?.data?.message || error.message,
      });
    }
  };
  
  if (isLoading) {
    return (
       <Card>
        <CardHeader>
          <CardTitle>{t('title')}</CardTitle>
          <CardDescription>
            {t('loadingDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
             <Skeleton className="h-10 w-full" />
             <Skeleton className="h-10 w-full" />
             <Skeleton className="h-10 w-full" />
             <Skeleton className="h-10 w-full" />
        </CardContent>
       </Card>
    );
  }

  if (!authStatus?.local_auth_enabled) {
      return (
        <Card>
            <CardHeader>
                <CardTitle>{t('disabled.title')}</CardTitle>
                <CardDescription>
                    {t('disabled.description')}
                </CardDescription>
            </CardHeader>
            <CardContent>
                <p className="text-sm text-muted-foreground">
                    {t('disabled.instruction')}
                </p>
                <Button asChild className="mt-4 w-full">
                    <Link href="/login">{t('disabled.backToLogin')}</Link>
                </Button>
            </CardContent>
        </Card>
      );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>
          {t('description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('form.username')}</FormLabel>
                  <FormControl>
                    <Input placeholder="unique_username" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
             <FormField
              control={form.control}
              name="nickname"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('form.nickname')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('form.nicknamePlaceholder')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('form.password')}</FormLabel>
                  <FormControl>
                    <Input type="password" placeholder="********" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button
              type="submit"
              className="w-full"
              disabled={form.formState.isSubmitting}
            >
              {form.formState.isSubmitting ? t('form.creatingAccount') : t('form.registerButton')}
            </Button>
          </form>
        </Form>
        <div className="mt-4 text-center text-sm">
          {t('alreadyHaveAccount')}{" "}
          <Link href="/login" className="underline">
            {t('loginLink')}
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}