// Registers the photoCropper Alpine component used by the profile photo upload.
// Mirrors reference/resources/js/cropper.js, but reads Cropper from the global
// UMD build (cropper.min.js) since this repo has no JS bundler.
function photoCropper() {
  return {
    preview: null,
    cropping: false,
    cropSrc: null,
    sourceType: 'image/jpeg',
    cropper: null,

    pick() {
      this.$refs.input.click();
    },

    onSelect(event) {
      const file = event.target.files[0];
      if (!file) {
        return;
      }

      this.sourceType = file.type || 'image/jpeg';

      if (this.cropSrc) {
        URL.revokeObjectURL(this.cropSrc);
      }

      this.cropSrc = URL.createObjectURL(file);
      this.cropping = true;

      this.$nextTick(() => this.initCropper());
    },

    initCropper() {
      if (this.cropper) {
        this.cropper.destroy();
      }

      this.cropper = new Cropper(this.$refs.cropImage, {
        viewMode: 1,
        dragMode: 'move',
        autoCropArea: 1,
        aspectRatio: 1,
        restore: false,
        guides: true,
        center: true,
        highlight: false,
        cropBoxMovable: true,
        cropBoxResizable: true,
        toggleDragModeOnDblclick: false,
        responsive: true,
      });
    },

    save() {
      if (!this.cropper) {
        return;
      }

      const outputType = this.sourceType === 'image/png' ? 'image/png' : 'image/jpeg';
      const canvas = this.cropper.getCroppedCanvas({
        maxWidth: 1200,
        maxHeight: 1200,
        imageSmoothingQuality: 'high',
      });

      canvas.toBlob((blob) => {
        if (!blob) {
          return;
        }

        const file = new File([blob], `photo.${outputType === 'image/png' ? 'png' : 'jpg'}`, { type: outputType });
        const transfer = new DataTransfer();

        transfer.items.add(file);
        this.$refs.input.files = transfer.files;

        if (this.preview) {
          URL.revokeObjectURL(this.preview);
        }

        this.preview = URL.createObjectURL(blob);
        this.close();
      }, outputType, 0.92);
    },

    cancel() {
      this.$refs.input.value = '';
      this.close();
    },

    zoomIn() {
      if (this.cropper) {
        this.cropper.zoom(0.1);
      }
    },

    zoomOut() {
      if (this.cropper) {
        this.cropper.zoom(-0.1);
      }
    },

    close() {
      if (this.cropper) {
        this.cropper.destroy();
        this.cropper = null;
      }

      if (this.cropSrc) {
        URL.revokeObjectURL(this.cropSrc);
        this.cropSrc = null;
      }

      this.cropping = false;
    },
  };
}

function registerPhotoCropper() {
  window.Alpine.data('photoCropper', photoCropper);
}

if (window.Alpine) {
  registerPhotoCropper();
} else {
  document.addEventListener('alpine:init', registerPhotoCropper);
}
