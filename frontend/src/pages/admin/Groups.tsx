import { Trash2, Users } from 'lucide-react';
import { useMemo, useState } from 'react';
import { CreateGroupDialog } from '@/components/admin/CreateGroupDialog';
import { GroupMembersDialog } from '@/components/admin/GroupMembersDialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useDeleteGroup, useGroups } from '@/hooks/useGroups';
import type { GroupWithMemberCount } from '@/types/models';

export const Groups = () => {
	const { data: groups, isLoading } = useGroups();
	const deleteMutation = useDeleteGroup();
	const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(
		null,
	);
	const [membersOf, setMembersOf] = useState<GroupWithMemberCount | null>(null);
	const [error, setError] = useState('');

	const rows = useMemo(() => groups ?? [], [groups]);

	const handleDelete = async () => {
		if (!confirm) return;
		setError('');
		try {
			await deleteMutation.mutateAsync(confirm.id);
			setConfirm(null);
		} catch (err) {
			setError(
				(err as { response?: { data?: { error?: string } } })?.response?.data
					?.error ?? 'Failed to delete group',
			);
		}
	};

	return (
		<div className="space-y-6">
			<div className="flex justify-between items-center">
				<div>
					<h1 className="text-3xl font-bold">Groups</h1>
					<p className="text-muted-foreground">
						Manage groups and grant permission to workspaces and registries.
					</p>
				</div>
				<CreateGroupDialog />
			</div>

			{error && (
				<div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
					{error}
				</div>
			)}

			<Card>
				<CardContent className="p-0">
					{isLoading ? (
						<div className="p-8 text-center text-muted-foreground">
							Loading…
						</div>
					) : rows.length === 0 ? (
						<div className="p-8 text-center text-muted-foreground">
							No groups yet.
						</div>
					) : (
						<div className="overflow-x-auto">
							<table className="w-full">
								<thead className="border-b bg-muted/50">
									<tr>
										<th className="text-left p-4 font-medium">Name</th>
										<th className="text-left p-4 font-medium">Description</th>
										<th className="text-left p-4 font-medium">Source</th>
										<th className="text-left p-4 font-medium">Members</th>
										<th className="text-left p-4 font-medium">Created</th>
										<th className="text-right p-4 font-medium">Actions</th>
									</tr>
								</thead>
								<tbody>
									{rows.map((g) => (
										<tr
											key={g.id}
											className="border-b last:border-0 hover:bg-muted/50"
										>
											<td className="p-4 font-medium">{g.name}</td>
											<td className="p-4 text-sm text-muted-foreground">
												{g.description}
											</td>
											<td className="p-4">
												<Badge
													variant="outline"
													className={
														g.source === 'oidc'
															? 'border-blue-500/40 text-blue-500'
															: ''
													}
												>
													{g.source}
												</Badge>
											</td>
											<td className="p-4">{g.member_count}</td>
											<td className="p-4 text-sm text-muted-foreground">
												{new Date(g.created_at).toLocaleDateString()}
											</td>
											<td className="p-4">
												<div className="flex justify-end gap-2">
													<Button
														variant="ghost"
														size="sm"
														title="View members"
														onClick={() => setMembersOf(g)}
													>
														<Users className="h-4 w-4" />
													</Button>
													<Button
														variant="ghost"
														size="sm"
														title={
															g.source === 'oidc'
																? 'OIDC groups cannot be deleted'
																: 'Delete group'
														}
														disabled={g.source === 'oidc'}
														onClick={() =>
															setConfirm({ id: g.id, name: g.name })
														}
													>
														<Trash2 className="h-4 w-4" />
													</Button>
												</div>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					)}
				</CardContent>
			</Card>

			<ConfirmDialog
				open={!!confirm}
				onOpenChange={(o) => !o && setConfirm(null)}
				onConfirm={handleDelete}
				title="Delete group"
				description={`Delete group "${confirm?.name}"? Members lose all permissions granted via this group.`}
				confirmText="Delete"
				cancelText="Cancel"
				variant="destructive"
			/>

			{membersOf && (
				<GroupMembersDialog
					group={membersOf}
					open={!!membersOf}
					onOpenChange={(o) => !o && setMembersOf(null)}
				/>
			)}
		</div>
	);
};
