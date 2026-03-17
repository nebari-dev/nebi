const colors = [
  'bg-green-100 text-green-700',
  'bg-blue-100 text-blue-700',
  'bg-purple-100 text-purple-700',
  'bg-orange-100 text-orange-700',
  'bg-pink-100 text-pink-700',
];

export const UserBadge = ({ username }: { username: string }) => {
  const colorClass = colors[username.charCodeAt(0) % colors.length];
  const initial = username[0]?.toUpperCase() || '?';
  return (
    <div className="inline-flex items-center gap-1.5 bg-muted border border-border rounded-full pl-[3px] pr-2.5 py-[3px]">
      <div className={`h-4 w-4 rounded-full flex items-center justify-center text-[10px] font-semibold shrink-0 ${colorClass}`}>
        {initial}
      </div>
      <span className="text-xs font-semibold text-muted-foreground">{username}</span>
    </div>
  );
};
