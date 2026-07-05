"use client";
import { useState, useCallback, useRef, useEffect } from 'react';
import { useDropzone, FileWithPath } from 'react-dropzone';
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import { useRouter } from 'next/navigation';
import api from '@/lib/api';
import { UploadCloud, File as FileIcon, X, Info, FolderUp, FileUp, Code, Upload } from 'lucide-react';
import useSWR, { useSWRConfig } from 'swr';
import { Attempts } from '@/lib/types';
import { Skeleton } from '../ui/skeleton';
import Editor from "@monaco-editor/react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { minimatch } from "minimatch";
import { useTranslations } from 'next-intl';

interface SubmissionUploadFormProps {
    problemId: string;
    uploadLimits: {
        max_num: number;
        max_size: number;
        upload_form?: boolean;
        upload_files?: string[];
        editor?: boolean;
        editor_files?: string[];
    };
}

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function btoaUTF8(str: string) {
    const bytes = new TextEncoder().encode(str);
    let binary = "";
    bytes.forEach(b => binary += String.fromCharCode(b));
    return btoa(binary);
}

function AttemptsCounter({ problemId, onLimitReached }: { problemId: string, onLimitReached: (isReached: boolean) => void }) {
    const t = useTranslations('submissions.upload.attemptsCounter');
    const { data: attempts, isLoading } = useSWR<Attempts>(`/problems/${problemId}/attempts`, fetcher, {
        onSuccess: (data) => {
            if (data && data.remaining === 0) {
                onLimitReached(true);
            }
        }
    });

    if (isLoading) return <Skeleton className="h-5 w-24" />;
    if (!attempts) return null;

    return (
        <div className="text-sm text-muted-foreground flex items-center gap-1">
            <Info className="h-4 w-4" />
            <span>
                {attempts.limit ? t('label', { used: attempts.used, limit: attempts.limit}) : t('label_unlimited')}
                {attempts.remaining !== null && attempts.remaining <= 3 && attempts.remaining > 0 && (
                    <span className="font-bold text-yellow-500 ml-2">{t('remaining', { remaining: attempts.remaining })}</span>
                )}
                {attempts.remaining === 0 && (
                    <span className="font-bold text-destructive ml-2">{t('limitReached')}</span>
                )}
            </span>
        </div>
    );
}

const getLanguageForFile = (filename: string = '') => {
    const extension = filename.split('.').pop()?.toLowerCase();
    switch (extension) {
        case 'cpp': case 'cxx': case 'h': case 'hpp': return 'cpp';
        case 'c': return 'c';
        case 'py': return 'python';
        case 'java': return 'java';
        case 'js': return 'javascript';
        case 'ts': return 'typescript';
        case 'json': return 'json';
        case 'xml': return 'xml';
        case 'html': return 'html';
        case 'css': return 'css';
        case 'md': return 'markdown';
        default: return 'plaintext';
    }
};


export default function SubmissionUploadForm({ problemId, uploadLimits }: SubmissionUploadFormProps) {
    const t = useTranslations('submissions.upload'); // 使用 useTranslations
    const [isSubmitting, setIsSubmitting] = useState(false);
    const [isLimitReached, setIsLimitReached] = useState(false);
    const { toast } = useToast();
    const router = useRouter();
    const { mutate } = useSWRConfig();
    
    const showEditor = uploadLimits.editor === true;
    const showUploader = uploadLimits.upload_form === true;
    
    const defaultMode = showEditor ? 'editor' : 'upload';
    const [activeMode, setActiveMode] = useState(defaultMode);

    // --- State & logic for Uploader Mode ---
    const [files, setFiles] = useState<File[]>([]);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const folderInputRef = useRef<HTMLInputElement>(null);

    const uploadFiles = uploadLimits.upload_files || [];

    const addFiles = useCallback((newFiles: File[]) => {

        if (uploadFiles.length > 0) {
            newFiles = newFiles.filter(file => 
                uploadFiles.some(pattern => minimatch(((file as FileWithPath).path || (file as any).webkitRelativePath || file.name).replace(/^\/+/, "").replace(/^(\.\/)+/, ""), pattern))
            );
            if (newFiles.length === 0) {
                toast({ variant: 'destructive', title: 'No valid files', description: `No selected files match the allowed patterns: ${uploadFiles.join(', ')}` });
                return;
            }
        }

        const allFiles = [...files, ...newFiles];
        if (uploadLimits.max_num > 0 && allFiles.length > uploadLimits.max_num) {
            toast({ 
                variant: 'destructive', 
                title: t('uploader.tooManyFilesToast.title'), 
                description: t('uploader.tooManyFilesToast.description', { maxNum: uploadLimits.max_num }) 
            });
            return;
        }
        setFiles(prev => [...prev, ...newFiles]);
    }, [files, uploadLimits.max_num, toast, t]);

    const onDrop = useCallback((acceptedFiles: FileWithPath[]) => { addFiles(acceptedFiles); }, [addFiles]);
    const handleManualFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
        if (event.target.files) { addFiles(Array.from(event.target.files)); }
        if(event.target) event.target.value = '';
    };
    const { getRootProps, isDragActive } = useDropzone({ onDrop, noClick: true, noKeyboard: true });
    const removeFile = (fileToRemove: File) => { setFiles(files.filter(file => file !== fileToRemove)); };

    // --- State & logic for Editor Mode ---
    const editorFiles = uploadLimits.editor_files || [];
    const [activeEditorFile, setActiveEditorFile] = useState<string>(editorFiles[0] || '');
    const [fileContents, setFileContents] = useState<Record<string, string>>({});
    const [monacoTheme, setMonacoTheme] = useState('light');
    const localStorageKey = `csoj-editor-content-${problemId}`;

    useEffect(() => {
        if (typeof window !== "undefined") {
            setMonacoTheme(window.document.documentElement.classList.contains('dark') ? 'vs-dark' : 'light');
        }

        if (showEditor) {
            try {
                const savedContent = localStorage.getItem(localStorageKey);
                const parsedContent = savedContent ? JSON.parse(savedContent) : {};
                const initialContents = Object.fromEntries(editorFiles.map(file => [file, '']));
                setFileContents({ ...initialContents, ...parsedContent });
            } catch (e) {
                console.error("Failed to load editor content from localStorage", e);
                setFileContents(Object.fromEntries(editorFiles.map(file => [file, ''])));
            }
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [problemId, showEditor]);

    useEffect(() => {
        if (!showEditor || Object.keys(fileContents).length === 0) return;
        const handler = setTimeout(() => {
            localStorage.setItem(localStorageKey, JSON.stringify(fileContents));
        }, 500);
        return () => clearTimeout(handler);
    }, [fileContents, localStorageKey, showEditor]);


    const handleSubmit = async () => {
        let filesToSubmit: File[] = [];

        if (activeMode === 'editor') {
            filesToSubmit = Object.entries(fileContents)
                .filter(([name]) => editorFiles.includes(name))
                .map(([name, content]) => new File([content], name, { type: 'text/plain' }));

            if (filesToSubmit.every(f => f.size === 0)) {
                toast({ 
                    variant: 'destructive', 
                    title: t('editor.emptyContentToast.title'), 
                    description: t('editor.emptyContentToast.description') 
                });
                return;
            }
        } else { // activeMode === 'upload'
            filesToSubmit = files;
            if (filesToSubmit.length === 0) {
                toast({ 
                    variant: 'destructive', 
                    title: t('submission.noFilesToast.title') 
                });
                return;
            }
        }
        
        const totalSize = filesToSubmit.reduce((acc, file) => acc + file.size, 0);
        if (uploadLimits.max_size > 0 && totalSize > uploadLimits.max_size * 1024 * 1024) {
            toast({
                variant: 'destructive',
                title: t('submission.fileTooLargeToast.title'),
                description: t('submission.fileTooLargeToast.description', { maxSize: uploadLimits.max_size }),
            });
            return;
        }

        setIsSubmitting(true);
        const formData = new FormData();
        filesToSubmit.forEach(file => {
            const filePath = ((file as FileWithPath).path || (file as any).webkitRelativePath || file.name).replace(/^\/+/, "").replace(/^(\.\/)+/, "");
            formData.append('files', file, btoaUTF8(filePath));
        });

        try {
            const response = await api.post(`/problems/${problemId}/submit`, formData);
            const submissionId = response.data.data.submission_id;
            toast({
                title: t('submission.successToast.title'),
                description: t('submission.successToast.description', { submissionId }),
            });
            mutate(`/problems/${problemId}/attempts`);
            router.push(`/submissions?id=${submissionId}`);
        } catch (error: any) {
            toast({
                variant: 'destructive',
                title: t('submission.failureToast.title'),
                description: t.rich('submission.failureToast.description', { 
                    message: error.response?.data?.message || 'An unexpected error occurred.' 
                }),
            });
        } finally {
            setIsSubmitting(false);
        }
    };
    
    const renderEditor = () => (
        <div className="space-y-4">
            <Tabs value={activeEditorFile} onValueChange={setActiveEditorFile} className="w-full">
                <TabsList className="grid h-auto w-full gap-1" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
                    {editorFiles.map(filename => (
                        <TabsTrigger key={filename} value={filename}>{filename}</TabsTrigger>
                    ))}
                </TabsList>
                <div className="mt-4 rounded-md border overflow-hidden">
                   <Editor
                        height="40vh"
                        path={activeEditorFile}
                        language={getLanguageForFile(activeEditorFile)}
                        value={fileContents[activeEditorFile]}
                        onChange={(value) => setFileContents(prev => ({ ...prev, [activeEditorFile]: value || '' }))}
                        theme={monacoTheme}
                        options={{ minimap: { enabled: false }, scrollBeyondLastLine: false, automaticLayout: true }}
                    />
                </div>
            </Tabs>
        </div>
    );

    const renderUploader = () => (
        <div className="space-y-4">
            <div {...getRootProps()}
                className={`relative flex flex-col items-center justify-center p-6 border-2 border-dashed rounded-md transition-colors 
                ${isDragActive ? 'border-primary bg-primary/10' : 'border-border'}
                ${isLimitReached ? 'bg-muted opacity-50 cursor-not-allowed' : ''}`}
            >
                <input type="file" multiple ref={fileInputRef} onChange={handleManualFileSelect} style={{ display: 'none' }} disabled={isLimitReached} />
                <input type="file" {...({ webkitdirectory: "true" } as any)} ref={folderInputRef} onChange={handleManualFileSelect} style={{ display: 'none' }} disabled={isLimitReached}/>
                <UploadCloud className="h-10 w-10 text-muted-foreground" />
                <p className="mt-2 text-sm text-muted-foreground text-center">
                    {isDragActive ? t('uploader.dragActive') : t('uploader.dragAndDrop')}
                </p>
                <div className='mt-4 flex flex-col sm:flex-row gap-2 w-full'>
                    <Button type="button" variant="outline" onClick={() => fileInputRef.current?.click()} disabled={isLimitReached} className='w-full'>
                        <FileUp className="mr-2 h-4 w-4" /> {t('uploader.selectFilesButton')}
                    </Button>
                    <Button type="button" variant="outline" onClick={() => folderInputRef.current?.click()} disabled={isLimitReached} className='w-full'>
                        <FolderUp className="mr-2 h-4 w-4" /> {t('uploader.selectFolderButton')}
                    </Button>
                </div>
                <p className="text-xs text-muted-foreground mt-4">
                    {t('uploader.limits', { maxNum: uploadLimits.max_num, maxSize: uploadLimits.max_size })}
                </p>
            </div>
            {files.length > 0 && (
                <div className="space-y-2">
                    <h4 className="font-semibold">{t('uploader.selectedFilesHeader')}</h4>
                    <ul className="space-y-1 bg-muted p-3 rounded-md max-h-48 overflow-y-auto">
                        {files.map((file, index) => {
                            const displayPath = ((file as FileWithPath).path || (file as any).webkitRelativePath || file.name).replace(/^\/+/, "").replace(/^(\.\/)+/, "");
                            return (
                                <li key={`${displayPath}-${index}`} className="flex items-center justify-between text-sm">
                                    <span className="flex items-center gap-2 truncate">
                                        <FileIcon className="h-4 w-4 shrink-0"/>
                                        <span className="truncate" title={displayPath}>{displayPath}</span> 
                                        <span className="text-muted-foreground shrink-0">{t('uploader.fileSizeUnit', { size: (file.size / 1024).toFixed(2) })}</span>
                                    </span>
                                    <Button variant="ghost" size="icon" onClick={() => removeFile(file)} className="h-6 w-6 shrink-0">
                                        <X className="h-4 w-4" />
                                    </Button>
                                </li>
                            );
                        })}
                    </ul>
                </div>
            )}
        </div>
    );
    
    const isSubmitDisabled = isSubmitting || isLimitReached || (activeMode === 'upload' && files.length === 0);

    return (
        <div className="space-y-4">
            <AttemptsCounter problemId={problemId} onLimitReached={setIsLimitReached} />

            {showEditor && showUploader ? (
                <Tabs value={activeMode} onValueChange={setActiveMode} className="w-full">
                    <TabsList className="grid w-full grid-cols-2">
                        <TabsTrigger value="editor">{t('modeTabs.editor')}</TabsTrigger>
                        <TabsTrigger value="upload">{t('modeTabs.upload')}</TabsTrigger>
                    </TabsList>
                    <TabsContent value="editor" className="mt-4">{renderEditor()}</TabsContent>
                    <TabsContent value="upload" className="mt-4">{renderUploader()}</TabsContent>
                </Tabs>
            ) : showEditor ? (
                renderEditor()
            ) : showUploader ? (
                renderUploader()
            ) : (
                <div className="text-center text-muted-foreground p-4 border rounded-md">
                    {t('submission.disabledMessage')}
                </div>
            )}
            
            {(showEditor || showUploader) && (
                 <Button onClick={handleSubmit} disabled={isSubmitDisabled} className="w-full">
                    {isSubmitting ? t('submission.submittingButton') : t('submission.submitButton')}
                </Button>
            )}
        </div>
    );
}