import React, { useEffect, useState } from 'react';
import { Input, Button, ScrollList, ScrollItem } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { copy, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const EndpointTicker = ({ serverAddress, endpointItems, isMobile }) => {
  const { t } = useTranslation();
  const [endpointIndex, setEndpointIndex] = useState(0);

  useEffect(() => {
    const timer = setInterval(() => {
      setEndpointIndex((prev) => (prev + 1) % endpointItems.length);
    }, 3000);
    return () => clearInterval(timer);
  }, [endpointItems.length]);

  const handleCopy = async () => {
    const ok = await copy(serverAddress);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  return (
    <div className='flex flex-col md:flex-row items-center justify-center gap-4 w-full max-w-md'>
      <Input
        readonly
        value={serverAddress}
        className='flex-1 !rounded-full'
        size={isMobile ? 'default' : 'large'}
        suffix={
          <div className='flex items-center gap-2'>
            <ScrollList
              bodyHeight={32}
              style={{ border: 'unset', boxShadow: 'unset' }}
            >
              <ScrollItem
                mode='wheel'
                cycled={true}
                list={endpointItems}
                selectedIndex={endpointIndex}
                onSelect={({ index }) => setEndpointIndex(index)}
              />
            </ScrollList>
            <Button
              type='primary'
              onClick={handleCopy}
              icon={<IconCopy />}
              className='!rounded-full'
            />
          </div>
        }
      />
    </div>
  );
};

export default EndpointTicker;
