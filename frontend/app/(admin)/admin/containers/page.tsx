"use client"

import useSWR from 'swr';
import { Suspense, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import api from '@/lib/api';
import { Container, PaginatedResponse } from '@/lib/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import SubmissionStatusBadge from '@/components/shared/submission-status-badge';
import { format } from 'date-fns';
import { formatDateTime, formatShort } from '@/lib/date-utils';
import Link from 'next/link';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { PaginationControls } from '@/components/shared/pagination-controls';
import { RefreshIntervalSelector } from '@/components/shared/refresh-interval-selector';

const fetcher = (url: string) => api.get(url).then(res => res.data.data);

function ContainerDetails({ containerId }: { containerId: string }) {
    const { data: container, isLoading } = useSWR<Container>(`/admin/containers/${containerId}`, fetcher);
    if (isLoading || !container) return <Skeleton className="h-48 w-full" />;

    return (
        <div className="space-y-4 text-sm">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8 gap-y-4">
                <div><strong>Container ID:</strong> <span className="font-mono">{container.id}</span></div>
                <div><strong>Submission ID:</strong> <Link href={`/admin/submissions?id=${container.submission_id}`} className="font-mono text-primary hover:underline">{container.submission_id}</Link></div>
                <div><strong>User:</strong> {container.user ? <Link href={`/admin/users?id=${container.user.id}`} className="font-mono text-primary hover:underline">{container.user.nickname} (@{container.user.username})</Link> : "N/A"}</div>
                <div><strong>Status:</strong> <SubmissionStatusBadge status={container.status} /></div>
                <div><strong>Image:</strong> <span className="font-mono">{container.image}</span></div>
                <div><strong>Exit Code:</strong> {container.exit_code}</div>
                <div><strong>Started At:</strong> {container.started_at ? formatDateTime(container.started_at) : 'N/A'}</div>
                <div><strong>Finished At:</strong> {container.finished_at ? formatDateTime(container.finished_at) : 'N/A'}</div>
            </div>
            <div>
                <strong>Log File Path:</strong>
                <pre className="mt-2 bg-muted p-2 rounded-md font-mono text-xs whitespace-pre-wrap break-all">{container.log_file_path}</pre>
            </div>
        </div>
    )
}

function ContainerList() {
	const [page, setPage] = useState(1);
	const [filters, setFilters] = useState({ user_query: '', submission_id: '', status: '' });
	const [refreshInterval, setRefreshInterval] = useState(1000);

	const queryParams = new URLSearchParams({
		...filters,
		page: page.toString(),
		limit: '20'
	});
	const query = queryParams.toString();

	const { data, isLoading } = useSWR<PaginatedResponse<Container>>(
		`/admin/containers?${query}`,
		fetcher,
		{ refreshInterval: refreshInterval }
	);

	const handleFilterChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		setFilters({ ...filters, [e.target.name]: e.target.value });
		setPage(1); // Reset page to 1 when filters change
	};

	const containers = data?.items;

	return (
		<Card>
			<CardHeader>
				<CardTitle>Containers</CardTitle>
			</CardHeader>
			<CardContent>
				<div className="flex flex-col md:flex-row gap-2 justify-between mb-4">
					<div className="flex flex-col sm:flex-row gap-2 flex-grow">
						<Input name="submission_id" placeholder="Filter by Submission ID..." onChange={handleFilterChange} />
						<Input name="user_query" placeholder="Filter by User..." onChange={handleFilterChange} />
						<Input name="status" placeholder="Filter by Status (e.g. Success)..." onChange={handleFilterChange} />
					</div>
					<RefreshIntervalSelector
						defaultValue={refreshInterval}
						onIntervalChange={setRefreshInterval}
					/>
				</div>
				{isLoading && !data ? <Skeleton className="h-96 w-full" /> : (
					<>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Container ID</TableHead>
									<TableHead>Submission ID</TableHead>
                                    <TableHead>User</TableHead>
									<TableHead>Image</TableHead>
									<TableHead>Status</TableHead>
									<TableHead>Created At</TableHead>
									<TableHead>Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{containers?.map(c => (
									<TableRow key={c.id}>
										<TableCell className="font-mono">{c.id.substring(0, 8)}</TableCell>
										<TableCell>
											<Link href={`/admin/submissions?id=${c.submission_id}`} className="font-mono text-primary hover:underline">
												{c.submission_id.substring(0, 8)}
											</Link>
										</TableCell>
                                        <TableCell>
                                            {c.user ? (
												<Link href={`/admin/users?id=${c.user.id}`} className="hover:underline">
													{c.user.nickname}
												</Link>
											) : (
												<span className="text-muted-foreground font-mono">{c.user_id.substring(0,8)}...</span>
											)}
                                        </TableCell>
										<TableCell>{c.image}</TableCell>
										<TableCell><SubmissionStatusBadge status={c.status} /></TableCell>
										<TableCell>{formatShort(c.CreatedAt)}</TableCell>
										<TableCell>
											<Dialog>
												<DialogTrigger asChild>
													<Button variant="outline" size="sm">Details</Button>
												</DialogTrigger>
												<DialogContent className="sm:max-w-3xl">
													<DialogHeader>
														<DialogTitle>Container Details</DialogTitle>
													</DialogHeader>
													<ContainerDetails containerId={c.id} />
												</DialogContent>
											</Dialog>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						<PaginationControls
							currentPage={data?.current_page ?? 1}
							totalPages={data?.total_pages ?? 1}
							onPageChange={setPage}
						/>
					</>
				)}
			</CardContent>
		</Card>
	);
}

function ContainersPageContent() {
    return (
        <div className="space-y-6">
            <ContainerList />
        </div>
    );
}

function ContainersPage() {
    return (
        <Suspense fallback={<Skeleton className="h-screen w-full" />}>
            <ContainersPageContent />
        </Suspense>
    );
}

export default ContainersPage;
