import React from 'react';

const SectionHeader = ({ title, subtitle, center = true }) => {
  return (
    <div className={`mb-10 md:mb-14 ${center ? 'text-center' : ''}`}>
      {title && (
        <h2 className='text-2xl md:text-3xl lg:text-4xl font-bold text-semi-color-text-0 leading-tight'>
          {title}
        </h2>
      )}
      {subtitle && (
        <p className='text-base md:text-lg text-semi-color-text-2 mt-3 md:mt-4 max-w-2xl mx-auto'>
          {subtitle}
        </p>
      )}
    </div>
  );
};

export default SectionHeader;
