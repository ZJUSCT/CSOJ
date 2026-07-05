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
import { useAuth } from "@/hooks/use-auth";
import api from "@/lib/api";
import { useRouter } from "next/navigation";
import { useToast } from "@/hooks/use-toast";
import { Separator } from "../ui/separator";
import { SiGitlab } from "react-icons/si";
import useSWR from "swr";
import { AuthStatus } from "@/lib/types";
import { Skeleton } from "../ui/skeleton";
import { useTranslations } from "next-intl";

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

export function LoginForm() {
  const t = useTranslations('auth.login');
  const { login } = useAuth();
  const router = useRouter();
  const { toast } = useToast();
  const gitlabLoginUrl = `/api/v1/auth/gitlab/login`;
  
  // Define schema using t() for localized error messages
  const formSchema = z.object({
    username: z.string().min(1, t('form.usernameRequired')),
    password: z.string().min(1, t('form.passwordRequired')),
  });

  const { data: authStatus, isLoading } = useSWR<AuthStatus>('/auth/status', fetcher);

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      username: "",
      password: "",
    },
  });

  const onSubmit = async (values: z.infer<typeof formSchema>) => {
    try {
      const response = await api.post("/auth/local/login", values);
      if (response.data.code === 0 && response.data.data.token) {
        login(response.data.data.token);
        toast({ title: t('toast.successTitle') });
        router.push("/contests");
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
          <div className="relative my-4">
            <div className="absolute inset-0 flex items-center">
                <Separator />
            </div>
          </div>
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>
          {authStatus?.local_auth_enabled
            ? t('descriptionLocal')
            : t('descriptionExternal')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {authStatus?.local_auth_enabled && (
          <>
            <Form {...form}>
              <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                <FormField
                  control={form.control}
                  name="username"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('form.username')}</FormLabel>
                      <FormControl>
                        <Input placeholder="your_username" {...field} />
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
                  {form.formState.isSubmitting ? t('form.loggingIn') : t('form.loginButton')}
                </Button>
              </form>
            </Form>

            <div className="relative my-4">
              <div className="absolute inset-0 flex items-center">
                <Separator />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-card px-2 text-muted-foreground">
                  {t('separatorText')}
                </span>
              </div>
            </div>
          </>
        )}
        
        {/* GitLab Login Button - Assuming GitLab is always enabled for now */}
        <Button variant="outline" className="w-full" asChild>
          <a href={gitlabLoginUrl}>
            <SiGitlab className="mr-2 h-4 w-4" />
            {t('gitlabButton')}
          </a>
        </Button>

        {authStatus?.local_auth_enabled && (
          <div className="mt-4 text-center text-sm">
            {t('noAccount')}{" "}
            <Link href="/register" className="underline">
              {t('registerLink')}
            </Link>
          </div>
        )}
      </CardContent>
    </Card>
  );
}