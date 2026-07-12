"use client";

import { useState } from 'react';
import useSWR from 'swr';
import api from '@/lib/api';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Checkbox } from '@/components/ui/checkbox';
import { useToast } from '@/hooks/use-toast';
import { Save } from 'lucide-react';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

interface SettingsResponse {
    settings: Record<string, string>;
    boot: {
        listen: string;
        storage: { Database: string; SubmissionContent: string; SubmissionLog: string; UserAvatar: string };
        jwt_secret_present: boolean;
    };
}

function parseSetting<T>(settings: Record<string, string>, key: string, fallback: T): T {
    const raw = settings[key];
    if (!raw) return fallback;
    try { return JSON.parse(raw) as T; } catch { return fallback; }
}

function SettingsPage() {
    const { data, isLoading, mutate } = useSWR<SettingsResponse>('/admin/settings', fetcher);

    if (isLoading || !data) return <Skeleton className="h-96 w-full" />;

    return (
        <div className="space-y-6">
            <Card>
                <CardHeader>
                    <CardTitle>Boot Configuration (config.yaml - restart required to change)</CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-2 gap-4 text-sm">
                    <div><span className="text-muted-foreground">Listen:</span> {data.boot.listen}</div>
                    <div><span className="text-muted-foreground">Database:</span> {data.boot.storage.Database}</div>
                    <div><span className="text-muted-foreground">Submission Content:</span> {data.boot.storage.SubmissionContent}</div>
                    <div><span className="text-muted-foreground">Submission Log:</span> {data.boot.storage.SubmissionLog}</div>
                    <div><span className="text-muted-foreground">User Avatar:</span> {data.boot.storage.UserAvatar}</div>
                    <div><span className="text-muted-foreground">JWT Secret:</span> <Badge variant={data.boot.jwt_secret_present ? "default" : "destructive"}>{data.boot.jwt_secret_present ? "Set" : "MISSING"}</Badge></div>
                </CardContent>
            </Card>
            <Tabs defaultValue="general">
                <TabsList className="grid w-full grid-cols-5">
                    <TabsTrigger value="general">General</TabsTrigger>
                    <TabsTrigger value="security">Security</TabsTrigger>
                    <TabsTrigger value="gitlab">GitLab OIDC</TabsTrigger>
                    <TabsTrigger value="devpods">DevPods</TabsTrigger>
                    <TabsTrigger value="cors">CORS</TabsTrigger>
                </TabsList>
                <TabsContent value="general"><GeneralTab settings={data.settings} onSaved={() => mutate()} /></TabsContent>
                <TabsContent value="security"><SecurityTab settings={data.settings} onSaved={() => mutate()} /></TabsContent>
                <TabsContent value="gitlab"><GitLabTab settings={data.settings} onSaved={() => mutate()} /></TabsContent>
                <TabsContent value="devpods">
                    <div className="space-y-4">
                        <DevPodGatewayTab settings={data.settings} onSaved={() => mutate()} />
                        <DevPodQuotaCard settings={data.settings} onSaved={() => mutate()} />
                    </div>
                </TabsContent>
                <TabsContent value="cors"><CORSTab settings={data.settings} onSaved={() => mutate()} /></TabsContent>
            </Tabs>
        </div>
    );
}

function GeneralTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const logger = parseSetting(settings, 'logger', { level: 'info', file: '' });
    const [level, setLevel] = useState(logger.level);
    const [file, setFile] = useState(logger.file);

    const handleSave = function() {
        api.put('/admin/settings/logger', { value: { level, file } }).then(function() {
            toast({ title: 'Logger updated', description: 'Restart required for the change to take effect.' });
            onSaved();
        }).catch(function() {
            toast({ variant: 'destructive', title: 'Update failed' });
        });
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>Logger <Badge variant="secondary">restart required</Badge></CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>Log Level</Label>
                    <Select value={level} onValueChange={setLevel}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                            <SelectItem value="debug">debug</SelectItem>
                            <SelectItem value="info">info</SelectItem>
                            <SelectItem value="warn">warn</SelectItem>
                            <SelectItem value="error">error</SelectItem>
                        </SelectContent>
                    </Select>
                </div>
                <div>
                    <Label>Log File (optional, empty = stdout only)</Label>
                    <Input value={file} onChange={e => setFile(e.target.value)} placeholder="csoj.log" />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function DevPodGatewayTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const gateway = parseSetting(settings, 'devpods.gateway', { host: '', port: 22, hostname_suffix: '', audit_namespace: 'devpod-system' });
    const [host, setHost] = useState(gateway.host);
    const [port, setPort] = useState(gateway.port);
    const [hostnameSuffix, setHostnameSuffix] = useState(gateway.hostname_suffix ?? '');
    const [auditNamespace, setAuditNamespace] = useState(gateway.audit_namespace || 'devpod-system');

    const handleSave = function() {
        const normalizedHost = host.trim();
        if (!normalizedHost) {
            toast({ variant: 'destructive', title: 'Host is required' });
            return;
        }
        if (!Number.isInteger(port) || port < 1 || port > 65535) {
            toast({ variant: 'destructive', title: 'Port must be between 1 and 65535' });
            return;
        }
        const normalizedSuffix = hostnameSuffix.trim();
        if (normalizedSuffix && !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(normalizedSuffix)) {
            toast({ variant: 'destructive', title: 'Hostname suffix must be a lowercase DNS label' });
            return;
        }
        const normalizedAuditNamespace = auditNamespace.trim();
        if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(normalizedAuditNamespace)) {
            toast({ variant: 'destructive', title: 'Audit namespace must be a lowercase DNS label' });
            return;
        }
        api.put('/admin/settings/devpods.gateway', { value: { host: normalizedHost, port, hostname_suffix: normalizedSuffix, audit_namespace: normalizedAuditNamespace } }).then(function() {
            setHost(normalizedHost);
            setHostnameSuffix(normalizedSuffix);
            setAuditNamespace(normalizedAuditNamespace);
            toast({ title: 'DevPod gateway settings updated' });
            onSaved();
        }).catch(function(error) {
            toast({ variant: 'destructive', title: 'Update failed', description: error.response?.data?.message });
        });
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>DevPod SSH Gateway <Badge variant="default">live</Badge></CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label htmlFor="devpod-gateway-host">Host</Label>
                    <Input
                        id="devpod-gateway-host"
                        value={host}
                        onChange={e => setHost(e.target.value)}
                        placeholder="devpod.example.com or 172.28.0.0"
                    />
                    <p className="mt-1 text-xs text-muted-foreground">Public DNS name or IP address users connect to. Do not include ssh://.</p>
                </div>
                <div>
                    <Label htmlFor="devpod-gateway-port">Port</Label>
                    <Input
                        id="devpod-gateway-port"
                        type="number"
                        min={1}
                        max={65535}
                        value={port}
                        onChange={e => setPort(Number(e.target.value))}
                        placeholder="22"
                    />
                </div>
                <div>
                    <Label htmlFor="devpod-gateway-audit-namespace">Audit Log Namespace</Label>
                    <Input
                        id="devpod-gateway-audit-namespace"
                        value={auditNamespace}
                        onChange={e => setAuditNamespace(e.target.value)}
                        placeholder="devpod-system"
                    />
                    <p className="mt-1 text-xs text-muted-foreground">Namespace containing Pods labeled app.kubernetes.io/name=devpod-gateway.</p>
                </div>
                <div>
                    <Label htmlFor="devpod-gateway-hostname-suffix">Hostname Suffix (optional)</Label>
                    <Input
                        id="devpod-gateway-hostname-suffix"
                        value={hostnameSuffix}
                        onChange={e => setHostnameSuffix(e.target.value)}
                        placeholder="hpc101"
                    />
                    <p className="mt-1 text-xs text-muted-foreground">Appended to the SSH login for an outer proxy, for example owner+devpod+hpc101.</p>
                </div>
                <div className="rounded-md bg-muted p-3 text-xs">
                    SSH commands will use <code>ssh user+devpod{hostnameSuffix.trim() ? `+${hostnameSuffix.trim()}` : ''}@{host.trim() || '<host>'} -p {port || '<port>'}</code>.
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function DevPodQuotaCard({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const configuredLimit = parseSetting(settings, 'devpods.max_per_user', 0);
    const [maxPerUser, setMaxPerUser] = useState(configuredLimit);

    const handleSave = function() {
        if (!Number.isInteger(maxPerUser) || maxPerUser < 0) {
            toast({ variant: 'destructive', title: 'Maximum must be a non-negative integer' });
            return;
        }
        api.put('/admin/settings/devpods.max_per_user', { value: maxPerUser }).then(function() {
            toast({ title: 'Global DevPod running limit updated' });
            onSaved();
        }).catch(function(error) {
            toast({ variant: 'destructive', title: 'Update failed', description: error.response?.data?.message });
        });
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>Global DevPod Running Limit <Badge variant="default">live</Badge></CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label htmlFor="devpod-max-per-user">Maximum Running DevPods per User</Label>
                    <Input
                        id="devpod-max-per-user"
                        type="number"
                        min={0}
                        step={1}
                        value={maxPerUser}
                        onChange={e => setMaxPerUser(Number(e.target.value))}
                    />
                    <p className="mt-1 text-xs text-muted-foreground">
                        Same concurrent running limit for every user, counted across all templates and DevPod clusters. Stopped DevPods do not count. Set to 0 for unlimited.
                    </p>
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function SecurityTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const local = parseSetting(settings, 'auth.local', { enabled: true });
    const [localEnabled, setLocalEnabled] = useState(local.enabled);
    const expireHours = parseSetting(settings, 'auth.jwt.expire_hours', 72);
    const [expire, setExpire] = useState(expireHours);

    const handleSaveLocal = function() {
        api.put('/admin/settings/auth.local', { value: { enabled: localEnabled } }).then(function() {
            toast({ title: 'Local auth setting updated' });
            onSaved();
        }).catch(function() {
            toast({ variant: 'destructive', title: 'Update failed' });
        });
    };

    const handleSaveExpire = function() {
        api.put('/admin/settings/auth.jwt.expire_hours', { value: expire }).then(function() {
            toast({ title: 'JWT expiry updated' });
            onSaved();
        }).catch(function() {
            toast({ variant: 'destructive', title: 'Update failed' });
        });
    };

    return (
        <div className="space-y-4">
            <Card>
                <CardHeader>
                    <CardTitle>Local Authentication <Badge variant="default">live</Badge></CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                    <div className="flex items-center space-x-2">
                        <Checkbox id="local-enabled" checked={localEnabled} onCheckedChange={(v) => setLocalEnabled(!!v)} />
                        <Label htmlFor="local-enabled">Enable local auth</Label>
                    </div>
                    <Button onClick={handleSaveLocal}><Save className="h-4 w-4 mr-2" /> Save</Button>
                </CardContent>
            </Card>
            <Card>
                <CardHeader>
                    <CardTitle>JWT Expiry <Badge variant="default">live</Badge></CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                    <div>
                        <Label>Expire Hours</Label>
                        <Input type="number" value={expire} onChange={e => setExpire(parseInt(e.target.value) || 72)} />
                    </div>
                    <Button onClick={handleSaveExpire}><Save className="h-4 w-4 mr-2" /> Save</Button>
                </CardContent>
            </Card>
        </div>
    );
}

function GitLabTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const gl = parseSetting(settings, 'auth.gitlab', { url: '', client_id: '', client_secret: '', redirect_uri: '', frontend_callback_url: '' });
    const [form, setForm] = useState(gl);

    const handleSave = function() {
        api.put('/admin/settings/auth.gitlab', { value: form }).then(function() {
            toast({ title: 'GitLab OIDC settings updated' });
            onSaved();
        }).catch(function() {
            toast({ variant: 'destructive', title: 'Update failed' });
        });
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>GitLab OIDC <Badge variant="default">live</Badge></CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>OIDC Provider URL</Label>
                    <Input value={form.url} onChange={e => setForm({ ...form, url: e.target.value })} placeholder="https://gitlab.com" />
                </div>
                <div>
                    <Label>Client ID</Label>
                    <Input value={form.client_id} onChange={e => setForm({ ...form, client_id: e.target.value })} />
                </div>
                <div>
                    <Label>Client Secret</Label>
                    <Input type="password" value={form.client_secret} onChange={e => setForm({ ...form, client_secret: e.target.value })} />
                </div>
                <div>
                    <Label>Redirect URI</Label>
                    <Input value={form.redirect_uri} onChange={e => setForm({ ...form, redirect_uri: e.target.value })} placeholder="https://oj.example.com/api/v1/auth/gitlab/callback" />
                </div>
                <div>
                    <Label>Frontend Callback URL</Label>
                    <Input value={form.frontend_callback_url} onChange={e => setForm({ ...form, frontend_callback_url: e.target.value })} placeholder="/callback" />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

function CORSTab({ settings, onSaved }: { settings: Record<string, string>; onSaved: () => void }) {
    const { toast } = useToast();
    const cors = parseSetting(settings, 'cors', { allowed_origins: [] as string[] });
    const [origins, setOrigins] = useState(cors.allowed_origins.join('\n'));

    const handleSave = function() {
        const list = origins.split('\n').map(function(s) { return s.trim(); }).filter(Boolean);
        api.put('/admin/settings/cors', { value: { allowed_origins: list } }).then(function() {
            toast({ title: 'CORS settings updated' });
            onSaved();
        }).catch(function() {
            toast({ variant: 'destructive', title: 'Update failed' });
        });
    };

    return (
        <Card>
            <CardHeader>
                <CardTitle>CORS <Badge variant="default">live</Badge></CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
                <div>
                    <Label>Allowed Origins</Label>
                    <Textarea className="font-mono text-sm min-h-[100px]" value={origins} onChange={e => setOrigins(e.target.value)} placeholder="https://oj.example.com" />
                </div>
                <Button onClick={handleSave}><Save className="h-4 w-4 mr-2" /> Save</Button>
            </CardContent>
        </Card>
    );
}

export default SettingsPage;
