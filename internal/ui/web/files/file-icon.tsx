import {File, FileCode, FileText} from 'lucide-react';
import {language} from './language.ts';

export function FileIcon({name}: {name: string}) {
  const lang = language(name);
  const Component = lang === 'markdown' || lang === 'plaintext' ? FileText : lang ? FileCode : File;
  return <Component className="file-icon size-4 shrink-0" aria-hidden="true" />;
}
