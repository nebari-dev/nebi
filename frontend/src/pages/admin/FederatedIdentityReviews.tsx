import {
  Check,
  Fingerprint,
  Loader2,
  ShieldAlert,
  UserRound,
  X,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  useApproveFederatedIdentityReview,
  useFederatedIdentityReviews,
  useRejectFederatedIdentityReview,
} from '@/hooks/useAdmin';
import {
  useApproveRemoteFederatedIdentityReview,
  useRejectRemoteFederatedIdentityReview,
  useRemoteFederatedIdentityReviews,
  useRemoteServer,
} from '@/hooks/useRemote';
import { useModeStore } from '@/store/modeStore';
import { useViewModeStore } from '@/store/viewModeStore';
import {
  type FederatedIdentityReview,
  isPendingFederatedIdentityReview,
} from '@/types';

const reviewDisplayName = (review: FederatedIdentityReview) =>
  review.username || review.name || review.email || review.subject;

const formatDate = (value: string) =>
  value ? new Date(value).toLocaleString() : '—';

const mutationErrorMessage = (err: unknown) => {
  const error = err as { response?: { data?: { error?: string } } };
  return (
    error?.response?.data?.error ||
    'Could not update the identity review. Please try again.'
  );
};

export const FederatedIdentityReviews = () => {
  const { data: reviews, isLoading: reviewsLoading } =
    useFederatedIdentityReviews();
  const approveReview = useApproveFederatedIdentityReview();
  const rejectReview = useRejectFederatedIdentityReview();

  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const viewMode = useViewModeStore((state) => state.viewMode);
  const { data: serverStatus } = useRemoteServer();
  const isRemoteConnected = isLocalMode && serverStatus?.status === 'connected';
  const shouldShowRemote = isRemoteConnected && viewMode === 'remote';

  const { data: remoteReviews, isLoading: remoteLoading } =
    useRemoteFederatedIdentityReviews(shouldShowRemote);
  const approveRemoteReview = useApproveRemoteFederatedIdentityReview();
  const rejectRemoteReview = useRejectRemoteFederatedIdentityReview();

  const displayedReviews = useMemo(() => {
    if (!shouldShowRemote) {
      return reviews || [];
    }
    return remoteReviews || [];
  }, [reviews, remoteReviews, shouldShowRemote]);

  const isLoading = reviewsLoading || (shouldShowRemote && remoteLoading);
  const isMutating =
    approveReview.isPending ||
    approveRemoteReview.isPending ||
    rejectReview.isPending ||
    rejectRemoteReview.isPending;
  const [reviewToApprove, setReviewToApprove] =
    useState<FederatedIdentityReview | null>(null);
  const [reviewToReject, setReviewToReject] =
    useState<FederatedIdentityReview | null>(null);
  const [error, setError] = useState('');

  const handleApprove = async () => {
    if (!reviewToApprove) return;

    setError('');
    try {
      if (shouldShowRemote) {
        await approveRemoteReview.mutateAsync(reviewToApprove.id);
      } else {
        await approveReview.mutateAsync(reviewToApprove.id);
      }
      setReviewToApprove(null);
    } catch (err) {
      setError(mutationErrorMessage(err));
    }
  };

  const handleReject = async () => {
    if (!reviewToReject) return;

    setError('');
    try {
      if (shouldShowRemote) {
        await rejectRemoteReview.mutateAsync(reviewToReject.id);
      } else {
        await rejectReview.mutateAsync(reviewToReject.id);
      }
      setReviewToReject(null);
    } catch (err) {
      setError(mutationErrorMessage(err));
    }
  };

  if (isLoading) {
    return (
      <div className="flex h-96 items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Identity Reviews</h1>
        <p className="text-muted-foreground">
          Review blocked federated identity links
        </p>
      </div>

      {error && (
        <div className="rounded border border-red-500/20 bg-red-500/10 px-4 py-3 text-red-500">
          {error}
        </div>
      )}

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th className="p-4 text-left font-medium">Status</th>
                  <th className="p-4 text-left font-medium">Existing User</th>
                  <th className="p-4 text-left font-medium">
                    Incoming Identity
                  </th>
                  <th className="p-4 text-left font-medium">Collision</th>
                  <th className="p-4 text-left font-medium">Issuer</th>
                  <th className="p-4 text-left font-medium">Requested</th>
                  <th className="p-4 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {displayedReviews.map((review) => (
                  <tr
                    key={review.id}
                    className="border-b last:border-0 hover:bg-muted/50"
                  >
                    <td className="p-4">
                      {isPendingFederatedIdentityReview(review) ? (
                        <Badge className="border-amber-300 bg-amber-100 text-amber-800">
                          <ShieldAlert className="mr-1 h-3 w-3" />
                          Pending
                        </Badge>
                      ) : (
                        <Badge className="border-red-300 bg-red-100 text-red-800">
                          <X className="mr-1 h-3 w-3" />
                          Rejected
                        </Badge>
                      )}
                    </td>
                    <td className="p-4">
                      <div className="flex items-start gap-2">
                        <UserRound className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                        <div>
                          <p className="font-medium">
                            {review.user?.username || review.user_id}
                          </p>
                          <p className="text-sm text-muted-foreground">
                            {review.user?.email || review.user_id}
                          </p>
                        </div>
                      </div>
                    </td>
                    <td className="p-4">
                      <div className="max-w-[18rem]">
                        <p className="font-medium">
                          {reviewDisplayName(review)}
                        </p>
                        <div className="mt-1 flex flex-wrap items-center gap-2">
                          {review.email && (
                            <span className="text-sm text-muted-foreground">
                              {review.email}
                            </span>
                          )}
                          <Badge
                            variant="outline"
                            className={
                              review.email_verified
                                ? 'border-green-300 text-green-700'
                                : 'border-zinc-300 text-zinc-700'
                            }
                          >
                            {review.email_verified
                              ? 'Email verified'
                              : 'Email unverified'}
                          </Badge>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                          sub: {review.subject}
                        </p>
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge variant="outline">
                        {review.collision_field.replace(/_/g, ' ')}
                      </Badge>
                    </td>
                    <td className="p-4">
                      <div className="flex max-w-[20rem] items-start gap-2">
                        <Fingerprint className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                        <span className="break-all font-mono text-xs text-muted-foreground">
                          {review.issuer}
                        </span>
                      </div>
                    </td>
                    <td className="whitespace-nowrap p-4 text-sm text-muted-foreground">
                      {formatDate(review.created_at)}
                    </td>
                    <td className="p-4">
                      {isPendingFederatedIdentityReview(review) ? (
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => setReviewToReject(review)}
                            disabled={isMutating}
                            aria-label={`Reject identity review for ${reviewDisplayName(review)}`}
                          >
                            {isMutating ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <X className="h-4 w-4" />
                            )}
                            Reject
                          </Button>
                          <Button
                            size="sm"
                            onClick={() => setReviewToApprove(review)}
                            disabled={isMutating}
                            aria-label={`Approve identity review for ${reviewDisplayName(review)}`}
                          >
                            {isMutating ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Check className="h-4 w-4" />
                            )}
                            Approve
                          </Button>
                        </div>
                      ) : (
                        <p className="text-right text-sm text-muted-foreground">
                          Rejected
                        </p>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {displayedReviews.length === 0 && (
        <div className="py-12 text-center">
          <p className="text-muted-foreground">No identity reviews</p>
        </div>
      )}

      <ConfirmDialog
        open={!!reviewToApprove}
        onOpenChange={(open) => !open && setReviewToApprove(null)}
        onConfirm={handleApprove}
        title="Approve Identity Link"
        description={
          reviewToApprove
            ? `Approve ${reviewDisplayName(reviewToApprove)} for ${reviewToApprove.user?.username || reviewToApprove.user_id}?`
            : ''
        }
        confirmText="Approve Link"
        cancelText="Cancel"
      />

      <ConfirmDialog
        open={!!reviewToReject}
        onOpenChange={(open) => !open && setReviewToReject(null)}
        onConfirm={handleReject}
        title="Reject Identity Link"
        description={
          reviewToReject
            ? `Reject ${reviewDisplayName(reviewToReject)} for ${reviewToReject.user?.username || reviewToReject.user_id}? Future OAuth login attempts for this identity will show a rejected request message.`
            : ''
        }
        confirmText="Reject Request"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
