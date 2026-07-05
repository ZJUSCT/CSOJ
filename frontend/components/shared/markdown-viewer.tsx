import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeMathjax from 'rehype-mathjax';
import rehypeSlug from 'rehype-slug';
import rehypeAutolinkHeadings from 'rehype-autolink-headings';
import { useState, useEffect } from 'react';
import api from '@/lib/api';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';

interface MarkdownViewerProps {
    content: string;
    assetContext?: 'contest' | 'problem';
    assetContextId?: string;
}

export default function MarkdownViewer({ content, assetContext, assetContextId }: MarkdownViewerProps) {
    const [processedContent, setProcessedContent] = useState<string>(content);
    const { toast } = useToast();

    const preprocessMarkdown = async (md: string): Promise<string> => {
        if (!assetContext || !assetContextId) return md;

        const regex = /(\]\(|!\[.*?\]\()(\.\/index\.assets\/[^\)]+|index\.assets\/[^\)]+)/g;
        const matches = Array.from(md.matchAll(regex));

        let result = md;
        for (const match of matches) {
            const uri = match[2];
            const assetPath = uri.replace('./', '');
            try {
                const res = await api.get('/assets/query_url', {
                    params: {
                        asset: `/api/v1/assets/${assetContext}s/${assetContextId}/${assetPath}`,
                    },
                });
                const newUrl = res.data.data.url;
                if (newUrl) {
                    result = result.replace(uri, newUrl);
                }
            } catch (e) {
                toast({
                    title: 'Assets Load Failed',
                    description: `Failed to load asset: ${assetPath}`,
                    variant: 'destructive',
                });
            }
        }
        return result;
    };

    useEffect(() => {
        preprocessMarkdown(content).then(setProcessedContent);
    }, [content, assetContext, assetContextId]);

    return (
        <div className={cn("prose prose-tight dark:prose-invert max-w-none leading-normal")}>
            <ReactMarkdown
                remarkPlugins={[remarkGfm, remarkMath]}
                rehypePlugins={[
                    rehypeMathjax,
                    rehypeSlug,
                    rehypeAutolinkHeadings
                ]}
            >
                {processedContent}
            </ReactMarkdown>
        </div>
    );
}