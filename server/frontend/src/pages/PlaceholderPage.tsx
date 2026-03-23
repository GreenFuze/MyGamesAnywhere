export function PlaceholderPage({ title, body }: { title: string; body: string }) {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="text-mga-muted">{body}</p>
    </div>
  )
}
