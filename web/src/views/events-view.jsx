import { EmptyBlock, ErrorBlock, LoadingBlock, StatCard, StatusBadge } from '@/components/app/display-primitives';
import { Badge } from '@/components/ui/badge';
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useI18n } from '@/i18n';
import { compactValue, formatDateTime } from '@/lib/workspace-formatters';

export function EventsView({ viewState, events, filteredLogs, eventSearch, setEventSearch }) {
  const { t } = useI18n();
  const processedEvents = events.filter((event) => Boolean(event.processedAt)).length;
  const pendingEvents = events.length - processedEvents;
  const highlightedLogs = filteredLogs.filter((entry) => ['error', 'warn'].includes(String(entry.level || '').toLowerCase())).length;
  const activeSearch = eventSearch.trim();

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label={t('events.stats.deliveries.label')} value={events.length} note={t('events.stats.deliveries.note')} />
        <StatCard
          label={t('events.stats.processed.label')}
          value={processedEvents}
          note={t('events.stats.processed.note')}
          accent={processedEvents > 0}
        />
        <StatCard
          label={t('events.stats.pending.label')}
          value={pendingEvents}
          note={t('events.stats.pending.note')}
          accent={pendingEvents > 0}
        />
        <StatCard
          label={t('events.stats.visibleLogs.label')}
          value={filteredLogs.length}
          note={t('events.stats.visibleLogs.note', {
            count: highlightedLogs,
            suffix: highlightedLogs === 1 ? '' : 's'
          })}
        />
      </div>

      <Card className="border bg-card/95">
        <Tabs defaultValue="deliveries" className="gap-0">
          <CardHeader className="border-b">
            <div>
              <CardTitle>{t('events.title')}</CardTitle>
              <CardDescription>{t('events.description')}</CardDescription>
            </div>
            <CardAction className="col-start-1 row-start-3 w-full max-w-none justify-self-stretch md:col-start-2 md:row-span-2 md:row-start-1 md:w-80 md:max-w-sm md:justify-self-end">
              <Input
                value={eventSearch}
                onChange={(event) => setEventSearch(event.target.value)}
                placeholder={t('events.searchPlaceholder')}
              />
            </CardAction>
          </CardHeader>
          <CardContent className="flex flex-col gap-4 pt-4">
            {viewState?.status === 'loading' ? (
              <LoadingBlock
                title={t('viewState.loading.title', { view: t('events.title') })}
                body={t('viewState.loading.body', { view: t('events.title') })}
              />
            ) : viewState?.status === 'error' ? (
              <ErrorBlock
                title={t('viewState.error.title', { view: t('events.title') })}
                body={viewState.error || t('viewState.error.body')}
              />
            ) : viewState?.status === 'empty' && !activeSearch ? (
              <EmptyBlock title={t('events.empty.title')} body={t('events.empty.body')} />
            ) : (
              <>
                <TabsList variant="line">
                  <TabsTrigger value="deliveries">{t('events.tabs.deliveries')}</TabsTrigger>
                  <TabsTrigger value="logs">{t('events.tabs.logs')}</TabsTrigger>
                </TabsList>

                <TabsContent value="deliveries" className="mt-0">
                  {events.length === 0 ? (
                    <EmptyBlock title={t('events.deliveries.empty.title')} body={t('events.deliveries.empty.body')} />
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('events.table.action')}</TableHead>
                          <TableHead>{t('events.table.repository')}</TableHead>
                          <TableHead>{t('events.table.deliveryId')}</TableHead>
                          <TableHead>{t('events.table.processed')}</TableHead>
                          <TableHead>{t('events.table.created')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {events.slice(0, 50).map((event) => (
                          <TableRow key={event.deliveryId}>
                            <TableCell><StatusBadge value={event.action} /></TableCell>
                            <TableCell>{event.repoOwner}/{event.repoName}</TableCell>
                            <TableCell className="font-mono text-xs text-muted-foreground">{compactValue(event.deliveryId)}</TableCell>
                            <TableCell>
                              <Badge variant={event.processedAt ? 'secondary' : 'outline'}>
                                {event.processedAt ? t('events.table.processedValue.ready') : t('events.table.processedValue.pending')}
                              </Badge>
                            </TableCell>
                            <TableCell>{formatDateTime(event.createdAt)}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </TabsContent>

                <TabsContent value="logs" className="mt-0">
                  {filteredLogs.length === 0 ? (
                    <EmptyBlock title={t('events.logs.empty.title')} body={t('events.logs.empty.body')} />
                  ) : (
                    <div className="flex flex-col gap-3">
                      {filteredLogs.slice(0, 60).map((entry) => (
                        <div key={entry.id} className="rounded-xl border bg-background/70 px-4 py-3">
                          <div className="flex items-start justify-between gap-4">
                            <div className="flex min-w-0 flex-col gap-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <StatusBadge value={entry.level} />
                                <span className="text-sm font-medium">{entry.message}</span>
                              </div>
                              <p className="text-sm text-muted-foreground">{entry.deliveryId || t('events.logs.systemEvent')}</p>
                            </div>
                            <p className="text-xs text-muted-foreground">{formatDateTime(entry.createdAt)}</p>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </TabsContent>
              </>
            )}
          </CardContent>
        </Tabs>
        <CardFooter className="justify-between gap-3 text-sm text-muted-foreground">
          <p>
            {activeSearch
              ? t('events.footer.filtering', { value: activeSearch })
              : t('events.footer.searchHint')}
          </p>
          <p>{t('events.footer.shown', { count: filteredLogs.length, suffix: filteredLogs.length === 1 ? '' : 's' })}</p>
        </CardFooter>
      </Card>
    </div>
  );
}
