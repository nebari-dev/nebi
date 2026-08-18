import {
  Check,
  Fingerprint,
  Loader2,
  ShieldAlert,
  Trash2,
  UserRound,
  X,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  useApproveFederatedIdentityReview,
  useDiscardFederatedIdentityReview,
  useFederatedIdentityReviews,
  useRejectFederatedIdentityReview,
} from '@/hooks/useAdmin';
import {
  useApproveRemoteFederatedIdentityReview,
  useDiscardRemoteFederatedIdentityReview,
  useRejectRemoteFederatedIdentityReview,
  useRemoteFederatedIdentityReviews,
  useRemoteView,
} from '@/hooks/useRemote';
import {
  type FederatedIdentityReview,
  type FederatedIdentityReviewStatusFilter,
  isPendingFederatedIdentityReview,
  type User,
} from '@/types';

type ReviewStatusTab = Exclude<FederatedIdentityReviewStatusFilter, 'all'>;

const reviewDisplayName = (review: FederatedIdentityReview) =>
  review.username || review.name || review.email || review.subject;

const formatDate = (value: string) =>
  value ? new Date(value).toLocaleString() : '—';

const hasAmbiguousCollision = (review: FederatedIdentityReview) =>
  !!review.collision_username_user_id &&
  !!review.collision_email_user_id &&
  review.collision_username_user_id !== review.collision_email_user_id;

const collisionLabel = (review: FederatedIdentityReview) => {
  if (hasAmbiguousCollision(review)) {
    return 'multiple users';
  }
  if (review.collision_field === 'username,email') {
    return 'username + email';
  }
  return review.collision_field.replace(/_/g, ' ');
};

const CollisionUser = ({
  label,
  user,
  userId,
}: {
  label: string;
  user?: User;
  userId?: string;
}) => {
  if (!user && !userId) return null;
  return (
    <div>
      <p className="text-xs font-medium uppercase text-muted-foreground">
        {label}
      </p>
      <p className="font-medium">{user?.username || 'Deleted user'}</p>
      <p className="break-all text-sm text-muted-foreground">
        {user?.email || userId}
      </p>
    </div>
  );
};

const ExistingCollisionUsers = ({
  review,
}: {
  review: FederatedIdentityReview;
}) => {
  if (hasAmbiguousCollision(review)) {
    return (
      <div className="space-y-3">
        <CollisionUser
          label="Username match"
          user={review.collision_username_user}
          userId={review.collision_username_user_id}
        />
        <CollisionUser
          label="Email match"
          user={review.collision_email_user}
          userId={review.collision_email_user_id}
        />
      </div>
    );
  }
  return (
    <div>
      <p className="font-medium">{review.user?.username || 'Deleted user'}</p>
      <p className="break-all text-sm text-muted-foreground">
        {review.user?.email || review.user_id}
      </p>
    </div>
  );
};

const mutationErrorMessage = (err: unknown) => {
  const error = err as { response?: { data?: { error?: string } } };
  return (
    error?.response?.data?.error ||
    'Could not update the identity review. Please try again.'
  );
};

export const FederatedIdentityReviews = () => {
  const [statusFilter, setStatusFilter] = useState<ReviewStatusTab>('pending');
  const { data: reviews, isLoading: reviewsLoading } =
    useFederatedIdentityReviews(statusFilter);
  const approveReview = useApproveFederatedIdentityReview();
  const rejectReview = useRejectFederatedIdentityReview();
  const discardReview = useDiscardFederatedIdentityReview();

  const { isRemoteView } = useRemoteView();

  const {
    data: remoteReviews,
    isLoading: remoteLoading,
    isError: remoteReviewsError,
    errorUpdateCount: remoteErrorUpdateCount,
  } = useRemoteFederatedIdentityReviews(isRemoteView, statusFilter);
  const approveRemoteReview = useApproveRemoteFederatedIdentityReview();
  const rejectRemoteReview = useRejectRemoteFederatedIdentityReview();
  const discardRemoteReview = useDiscardRemoteFederatedIdentityReview();

  const displayedReviews = useMemo(() => {
    if (!isRemoteView) {
      return reviews || [];
    }
    return remoteReviews || [];
  }, [reviews, remoteReviews, isRemoteView]);

  const isLoading = isRemoteView
    ? remoteLoading && remoteErrorUpdateCount === 0
    : reviewsLoading;
  const remoteUnreachable = isRemoteView && remoteReviewsError;
  const [reviewToApprove, setReviewToApprove] =
    useState<FederatedIdentityReview | null>(null);
  const [reviewToReject, setReviewToReject] =
    useState<FederatedIdentityReview | null>(null);
  const [reviewToDiscard, setReviewToDiscard] =
    useState<FederatedIdentityReview | null>(null);
  const [error, setError] = useState('');

  const handleApprove = async () => {
    if (!reviewToApprove) return;

    setError('');
    try {
      if (isRemoteView) {
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
      if (isRemoteView) {
        await rejectRemoteReview.mutateAsync(reviewToReject.id);
      } else {
        await rejectReview.mutateAsync(reviewToReject.id);
      }
      setReviewToReject(null);
    } catch (err) {
      setError(mutationErrorMessage(err));
    }
  };

  const handleDiscard = async () => {
    if (!reviewToDiscard) return;

    setError('');
    try {
      if (isRemoteView) {
        await discardRemoteReview.mutateAsync(reviewToDiscard.id);
      } else {
        await discardReview.mutateAsync(reviewToDiscard.id);
      }
      setReviewToDiscard(null);
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
        <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-3xl font-bold">Identity Reviews</h1>
            <p className="text-muted-foreground">
              Review blocked federated identity links
            </p>
          </div>
          <Tabs
            value={statusFilter}
            onValueChange={(value) => setStatusFilter(value as ReviewStatusTab)}
          >
            <TabsList>
              <TabsTrigger value="pending">Pending</TabsTrigger>
              <TabsTrigger value="rejected">Rejected</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </div>

      {remoteUnreachable && <RemoteUnreachableBanner />}

      {error && (
        <div className="rounded border border-red-500/20 bg-red-500/10 px-4 py-3 text-red-500">
          {error}
        </div>
      )}

      {!remoteUnreachable && (
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
                  {displayedReviews.map((review) => {
                    const isPending = isPendingFederatedIdentityReview(review);
                    const isAmbiguous = hasAmbiguousCollision(review);
                    const isApproving =
                      (isRemoteView
                        ? approveRemoteReview.variables
                        : approveReview.variables) === review.id &&
                      (isRemoteView
                        ? approveRemoteReview.isPending
                        : approveReview.isPending);
                    const isRejecting =
                      (isRemoteView
                        ? rejectRemoteReview.variables
                        : rejectReview.variables) === review.id &&
                      (isRemoteView
                        ? rejectRemoteReview.isPending
                        : rejectReview.isPending);
                    const isDiscarding =
                      (isRemoteView
                        ? discardRemoteReview.variables
                        : discardReview.variables) === review.id &&
                      (isRemoteView
                        ? discardRemoteReview.isPending
                        : discardReview.isPending);
                    const isRowMutating =
                      isApproving || isRejecting || isDiscarding;

                    return (
                      <tr
                        key={review.id}
                        className="border-b last:border-0 hover:bg-muted/50"
                      >
                        <td className="p-4">
                          {isPending ? (
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
                            <ExistingCollisionUsers review={review} />
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
                            {collisionLabel(review)}
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
                          {isPending ? (
                            <div className="flex justify-end gap-2">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => setReviewToReject(review)}
                                disabled={isRowMutating}
                                aria-label={`Reject identity review for ${reviewDisplayName(review)}`}
                              >
                                {isRejecting ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <X className="h-4 w-4" />
                                )}
                                Reject
                              </Button>
                              <Button
                                size="sm"
                                onClick={() => setReviewToApprove(review)}
                                disabled={isRowMutating || isAmbiguous}
                                title={
                                  isAmbiguous
                                    ? 'Resolve conflicting users before approving'
                                    : undefined
                                }
                                aria-label={`Approve identity review for ${reviewDisplayName(review)}`}
                              >
                                {isApproving ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Check className="h-4 w-4" />
                                )}
                                Approve
                              </Button>
                            </div>
                          ) : (
                            <div className="flex justify-end">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => setReviewToDiscard(review)}
                                disabled={isRowMutating}
                                aria-label={`Discard identity review for ${reviewDisplayName(review)}`}
                              >
                                {isDiscarding ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Trash2 className="h-4 w-4" />
                                )}
                                Discard
                              </Button>
                            </div>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {!remoteUnreachable && displayedReviews.length === 0 && (
        <div className="py-12 text-center">
          <p className="text-muted-foreground">
            No {statusFilter} identity reviews
          </p>
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

      <ConfirmDialog
        open={!!reviewToDiscard}
        onOpenChange={(open) => !open && setReviewToDiscard(null)}
        onConfirm={handleDiscard}
        title="Discard Identity Review"
        description={
          reviewToDiscard
            ? `Discard the review for ${reviewDisplayName(reviewToDiscard)}? The next OAuth login for this identity can create a fresh pending review if the collision still exists.`
            : ''
        }
        confirmText="Discard Review"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
