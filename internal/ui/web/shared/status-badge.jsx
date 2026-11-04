import {Badge} from '../components/ui/badge';

export const StatusBadge = ({kind, title}) => <Badge variant={kind === 'exited' ? 'destructive' : 'secondary'} title={title}>{title}</Badge>;
