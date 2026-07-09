"use client";

import { useState, useCallback } from 'react';
import useSWR from 'swr';
import api from '@/lib/api';
import { AssetFile } from '@/lib/types';
import { useToast } from '@/hooks/use-toast';
import { formatBytes } from '@/lib/utils';
import { format } from 'date-fns';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Button, buttonVariants } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Folder, File, Trash2, Upload, Download } from 'lucide-react';
import { Input } from '../ui/input';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '../ui/alert-dialog';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

interface AssetManagerProps {
    assetType: 'contest' | 'problem';
    assetId: string;
}

export function AssetManager({ assetType, assetId }: AssetManagerProps) {
    const apiPrefix = assetType === 'contest' ? `/admin/contests` : `/admin/problems`;
    const assetUrl = `${apiPrefix}/${assetId}/assets`;

    const { data: assets, error, isLoading, mutate } = useSWR<AssetFile[]>(assetUrl, fetcher);
    const { toast } = useToast();
    const [uploading, setUploading] = useState(false);
    const [selectedFiles, setSelectedFiles] = useState<FileList | null>(null);

    const handleUpload = async () => {
        if (!selectedFiles || selectedFiles.length === 0) {
            toast({ variant: 'destructive', title: 'No files selected' });
            return;
        }

        setUploading(true);
        const formData = new FormData();
        for (let i = 0; i < selectedFiles.length; i++) {
            formData.append('files', selectedFiles[i]);
        }

        try {
            await api.post(assetUrl, formData, {
                headers: { 'Content-Type': 'multipart/form-data' },
            });
            toast({ title: 'Upload successful' });
            setSelectedFiles(null);
            mutate();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Upload failed', description: err.response?.data?.message });
        } finally {
            setUploading(false);
        }
    };

    const handleDelete = async (path: string) => {
        try {
            await api.delete(assetUrl, { data: { path } });
            toast({ title: 'Asset deleted' });
            mutate();
        } catch (err: any) {
            toast({ variant: 'destructive', title: 'Delete failed', description: err.response?.data?.message });
        }
    };

    const handleDownload = useCallback(async (asset: AssetFile) => {
        const downloadUrl = `/admin/${assetType}s/${assetId}/assets/index.assets/${asset.path}`;
        try {
            toast({ title: `Preparing download for ${asset.name}...` });
            const response = await api.get(downloadUrl, {
                responseType: 'blob',
            });

            const blob = new Blob([response.data], { type: response.headers['content-type'] });
            const url = window.URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = url;
            link.setAttribute('download', asset.name);
            document.body.appendChild(link);
            link.click();

            document.body.removeChild(link);
            window.URL.revokeObjectURL(url);
        } catch (err: any) {
            let description = 'Could not download the file.';
            if (err.response?.data instanceof Blob) {
                try {
                    const errorText = await err.response.data.text();
                    const errorJson = JSON.parse(errorText);
                    description = errorJson.message || description;
                } catch (e) {
                    // Not a JSON error, stick with the default message.
                }
            } else if (err.response?.data?.message) {
                description = err.response.data.message;
            }
            toast({
                variant: 'destructive',
                title: 'Download Failed',
                description: description,
            });
        }
    }, [assetType, assetId, toast]);

    if (error) return <div>Failed to load assets.</div>;

    return (
        <Card>
            <CardHeader>
                <CardTitle>Asset Management</CardTitle>
            </CardHeader>
            <CardContent>
                <div className="mb-4 flex flex-col sm:flex-row gap-2 border p-4 rounded-lg">
                    <Input type="file" multiple onChange={(e) => setSelectedFiles(e.target.files)} className="flex-grow" />
                    <Button onClick={handleUpload} disabled={uploading || !selectedFiles}>
                        <Upload className="mr-2 h-4 w-4" /> {uploading ? 'Uploading...' : 'Upload'}
                    </Button>
                </div>
                {isLoading ? <Skeleton className="h-48 w-full" /> :
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>Name</TableHead>
                                <TableHead>Size</TableHead>
                                <TableHead>Modified</TableHead>
                                <TableHead className="text-right">Actions</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {assets?.map(asset => (
                                <TableRow key={asset.path}>
                                    <TableCell className="flex items-center gap-2 font-medium">
                                        {asset.is_dir ? <Folder className="text-blue-500" /> : <File className="text-muted-foreground" />}
                                        {asset.name}
                                    </TableCell>
                                    <TableCell>{!asset.is_dir ? formatBytes(asset.size) : '—'}</TableCell>
                                    <TableCell>{format(new Date(asset.mod_time), 'Pp')}</TableCell>
                                    <TableCell className="text-right">
                                        {!asset.is_dir && (
                                            <Button variant="ghost" size="icon" onClick={() => handleDownload(asset)} title="Download file">
                                                <Download className="h-4 w-4" />
                                            </Button>
                                        )}
                                        <AlertDialog>
                                            <AlertDialogTrigger asChild>
                                                <Button variant="ghost" size="icon" className="text-destructive hover:text-destructive">
                                                    <Trash2 className="h-4 w-4" />
                                                </Button>
                                            </AlertDialogTrigger>
                                            <AlertDialogContent>
                                                <AlertDialogHeader><AlertDialogTitle>Confirm Deletion</AlertDialogTitle></AlertDialogHeader>
                                                <AlertDialogDescription>
                                                    Are you sure you want to delete `{asset.path}`? This action cannot be undone.
                                                </AlertDialogDescription>
                                                <AlertDialogFooter>
                                                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                                                    <AlertDialogAction onClick={() => handleDelete(asset.path)} className={buttonVariants({ variant: 'destructive'})}>
                                                        Delete
                                                    </AlertDialogAction>
                                                </AlertDialogFooter>
                                            </AlertDialogContent>
                                        </AlertDialog>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                }
            </CardContent>
        </Card>
    );
}